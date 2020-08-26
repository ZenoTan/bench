package main

import (
	"context"
	"fmt"
	"github.com/prometheus/common/model"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

type bench interface {
	run() error
	collect() error
}

type scaleOut struct {
	c *cluster
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
		if mean * mean * 0.1 < dev {
			return false
		}
	}
	fmt.Printf("Result:\n%v\n", result)
	fmt.Println("Balanced")
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
	return "", nil
}

// lastReport is
func (s *scaleOut) mergeReport(lastReport, report string) (plainText string) {
	//todo @zeyuan
	return ""
}
