package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type bench interface {
	run() error
	collect() error
}

type timePoint struct {
	addTime     time.Time
	balanceTime time.Time
}

type stats struct {
	interval int
	balanceLeaderCount int
	balanceRegionCount int
	prevLatency int
	curLatency int
}

type scaleOut struct {
	c *cluster
	t timePoint
}

func newScaleOut(c *cluster) *scaleOut {
	return &scaleOut{
		c: c,
	}
}

func (s *scaleOut) run() error {
	if err := s.c.addStore(); err != nil {
		return err
	}
	s.t.addTime = time.Now()
	//for {
	//	//	time.Sleep(time.Minute)
	//	//	if s.isBalance() {
	//	//		return nil
	//	//	}
	//	//}
	s.isBalance()
	return nil
}

func (s *scaleOut) isBalance() bool {
	// todo get data from prometheus
	// todo @zeyuan
	client, err := api.NewClient(api.Config{
		Address: s.c.prometheus,
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := v1.Range{
		Start: time.Now().Add(-9 * time.Minute),
		End:   time.Now(),
		Step:  time.Minute,
	}
	result, warnings, err := v1api.QueryRange(ctx, "pd_scheduler_store_status{type=\"region_score\"}", r)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	matrix := result.(model.Matrix)
	for _, data := range matrix {
		if len(data.Values) != 10 {
			return false
		}
		mean := 0.0
		dev := 0.0
		for _, v := range data.Values {
			mean += float64(v.Value) / 10
		}
		for _, v := range data.Values {
			dev += (float64(v.Value) - mean) * (float64(v.Value) - mean) / 10
		}
		if mean*mean*0.1 < dev {
			return false
		}
	}
	fmt.Printf("Result:\n%v\n", result)
	fmt.Println("Balanced")
	s.t.balanceTime = time.Now()
	return true
}

func (s *scaleOut) collect() error {
	// create report
	report, err := s.createReport()
	if err != nil {
		return err
	}

	// try get last report
	lastReport, err := s.c.getLastReport()
	if err != nil {
		return err
	}

	// send report
	var plainText string
	if len(lastReport) == 0 { //first send
		plainText = ""
	} else { //second send
		plainText = s.mergeReport(lastReport, report)
	}

	return s.c.sendReport(report, plainText)
}

func (s *scaleOut) createReport() (string, error) {
	//todo @zeyuan
	rep := stats{interval: int(s.t.balanceTime.Sub(s.t.addTime).Seconds())}
	client, err := api.NewClient(api.Config{
		Address: s.c.prometheus,
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, warnings, err := v1api.Query(ctx,
		"sum(rate(tidb_server_handle_query_duration_seconds_sum{sql_type!=\"internal\"}[30s])) / " +
		"sum(rate(tidb_server_handle_query_duration_seconds_count{sql_type!=\"internal\"}[30s]))", s.t.addTime)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	fmt.Printf("Result:\n%v\n", result)
	bytes, err := json.Marshal(rep)
	return string(bytes), err
}

// lastReport is
func (s *scaleOut) mergeReport(lastReport, report string) (plainText string) {
	//todo @zeyuan
	last := stats{}
	cur := stats{}
	json.Unmarshal([]byte(lastReport), last)
	json.Unmarshal([]byte(report), cur)
	return ""
}
