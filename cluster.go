package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/pingcap/errors"
	"github.com/pingcap/log"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"go.uber.org/zap"
)

const (
	resourcePrefix = "api/cluster/resource/%v"
	scaleOutPrefix = "api/cluster/scale_out/%v/%v/%v"
	resultsPrefix  = "api/cluster/workload/%v/result"
)

type Spec struct {
	CPU  string
	Mem  string
	Disk string
}

type ResourceRequestItem struct {
	gorm.Model
	ItemID uint   `gorm:"column:item_id;unique;not null" json:"item_id"`
	Spec   Spec   `gorm:"column:spec;type:longtext;not null" json:"spec"`
	Status string `gorm:"column:status;type:varchar(255);not null" json:"status"`
	RRID   uint   `gorm:"column:rr_id;not null" json:"rr_id"`
	RID    uint   `gorm:"column:r_id;not null" json:"r_id"`
	// Components records which *_servers are serving on this machine
	Components string `gorm:"column:components" json:"components"`
}

func (r *ResourceRequestItem) isAvailable(toScale string) bool {
	components := strings.Split(r.Components, "|")
	for _, component := range components {
		if component == toScale {
			return false
		}
	}
	return true
}

type WorkloadReport struct {
	gorm.Model
	CRID      uint    `gorm:"column:cr_id;not null" json:"cr_id"`
	Data      string  `gorm:"column:result;type:longtext;not null" json:"data"`
	PlainText *string `gorm:"column:plaintext" json:"plaintext,omitempty"`
}

type Cluster struct {
	name       string
	tidb       string
	pd         string
	prometheus string
	api        string
	client     *http.Client
}

func NewCluster(name, tidb, pd, prometheus, api string) *Cluster {
	return &Cluster{
		name:       name,
		tidb:       tidb,
		pd:         pd,
		prometheus: prometheus,
		api:        api,
		client:     &http.Client{},
	}
}

func (c *Cluster) joinUrl(prefix string) string {
	return c.api + "/" + prefix
}

func (c *Cluster) getAvailableResourceID(component string) (uint, error) {
	// get all nodes
	prefix := fmt.Sprintf(resourcePrefix, c.name)
	url := c.joinUrl(prefix)
	resp, err := doRequest(url, http.MethodGet)
	if err != nil {
		return 0, err
	}
	resources := make([]ResourceRequestItem, 0, 0)
	err = json.Unmarshal([]byte(resp), &resources)
	if err != nil {
		return 0, err
	}

	// select available
	for _, resource := range resources {
		if resource.isAvailable(component) {
			return resource.ID, nil
		}
	}

	return 0, errors.New("no available resources")
}

func (c *Cluster) scaleOut(component string, id uint) error {
	prefix := fmt.Sprintf(scaleOutPrefix, c.name, id, component)
	url := c.joinUrl(prefix)
	_, err := doRequest(url, http.MethodPost)
	return err
}

func (c *Cluster) AddStore() error {
	component := "tikv"
	id, err := c.getAvailableResourceID(component)
	if err != nil {
		return err
	}
	return c.scaleOut(component, id)
}

func (c *Cluster) SendReport(data, plainText string) error {
	prefix := fmt.Sprintf(resultsPrefix, c.name)
	url := c.joinUrl(prefix)
	return postJSON(url, map[string]interface{}{
		"data":      data,
		"plaintext": plainText,
	})
}

func (c *Cluster) GetLastReport() (*WorkloadReport, error) {
	prefix := fmt.Sprintf(resultsPrefix, c.name)
	url := c.joinUrl(prefix)
	resp, err := doRequest(url, http.MethodGet)
	if err != nil {
		return nil, err
	}

	reports := make([]WorkloadReport, 0, 0)
	err = json.Unmarshal([]byte(resp), &reports)
	if err != nil || len(reports) == 0 {
		return nil, err
	}
	return &reports[0], nil
}

func (c *Cluster) getMetric(query string, t time.Time) float64 {
	client, err := api.NewClient(api.Config{
		Address: c.prometheus,
	})
	if err != nil {
		log.Error("error creating client", zap.Error(err))
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, warnings, err := v1api.Query(ctx, query, t)
	if err != nil {
		log.Error("error querying Prometheus", zap.Error(err))
	}
	if len(warnings) > 0 {
		log.Warn("query has warnings")
	}
	vector := result.(model.Vector)
	if len(vector) >= 1 {
		return float64(vector[0].Value)
	}
	return 0
}

func (c *Cluster) getMatrixMetric(query string, r v1.Range) [][]float64 {
	client, err := api.NewClient(api.Config{
		Address: c.prometheus,
	})
	if err != nil {
		log.Error("error creating client", zap.Error(err))
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, warnings, err := v1api.QueryRange(ctx, query, r)
	if err != nil {
		log.Error("error querying Prometheus", zap.Error(err))
	}
	if len(warnings) > 0 {
		log.Warn("query has warnings")
	}
	matrix := result.(model.Matrix)
	var ret [][]float64
	for _, m := range matrix {
		var r []float64
		for _, v := range m.Values {
			r = append(r, float64(v.Value))
		}
		ret = append(ret, r)
	}
 	return ret
}
