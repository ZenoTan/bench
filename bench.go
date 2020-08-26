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
	Interval               int     `json:"interval"`
	PrevBalanceLeaderCount int     `json:"prevBalanceLeaderCount"`
	PrevBalanceRegionCount int     `json:"prevBalanceRegionCount"`
	CurBalanceLeaderCount  int     `json:"curBalanceLeaderCount"`
	CurBalanceRegionCount  int     `json:"curBalanceRegionCount"`
	PrevLatency            float64 `json:"prevLatency"`
	CurLatency             float64 `json:"curLatency"`
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
	for {
		time.Sleep(time.Minute)
		if s.isBalance() {
			return nil
		}
	}
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
	fmt.Printf("Result:\n%v\n", result)
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
	fmt.Println("Balanced")
	s.t.balanceTime = time.Now()
	return true
}

func (s *scaleOut) collect() error {
	// create report data
	data, err := s.createReport()
	if err != nil {
		return err
	}

	// try get last data
	lastReportData, err := s.c.getLastReportData()
	if err != nil {
		return err
	}

	// send data
	var plainText string
	if len(lastReportData) == 0 { //first send
		plainText = ""
	} else { //second send
		plainText = s.mergeReport(lastReportData, data)
	}

	return s.c.sendReport(data, plainText)
}

func (s *scaleOut) createReport() (string, error) {
	//todo @zeyuan
	rep := &stats{Interval: int(s.t.balanceTime.Sub(s.t.addTime).Seconds())}
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
		"sum(rate(tidb_server_handle_query_duration_seconds_sum{sql_type!=\"internal\"}[30s])) / "+
			"sum(rate(tidb_server_handle_query_duration_seconds_count{sql_type!=\"internal\"}[30s]))", s.t.addTime)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	fmt.Printf("Result:\n%v\n", result)
	vector := result.(model.Vector)
	if len(vector) >= 1 {
		rep.PrevLatency = float64(vector[0].Value)
	}

	result, warnings, err = v1api.Query(ctx,
		"sum(rate(tidb_server_handle_query_duration_seconds_sum{sql_type!=\"internal\"}[30s])) / "+
			"sum(rate(tidb_server_handle_query_duration_seconds_count{sql_type!=\"internal\"}[30s]))", s.t.balanceTime)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	fmt.Printf("Result:\n%v\n", result)
	vector = result.(model.Vector)
	if len(vector) >= 1 {
		rep.CurLatency = float64(vector[0].Value)
	}

	result, warnings, err = v1api.Query(ctx,
		"pd_scheduler_event_count{type=\"balance-leader-scheduler\", name=\"schedule\"}", s.t.balanceTime)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	fmt.Printf("Result:\n%v\n", result)
	vector = result.(model.Vector)
	if len(vector) >= 1 {
		rep.CurBalanceLeaderCount = int(vector[0].Value)
	}

	result, warnings, err = v1api.Query(ctx,
		"pd_scheduler_event_count{type=\"balance-region-scheduler\", name=\"schedule\"}", s.t.balanceTime)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	fmt.Printf("Result:\n%v\n", result)
	vector = result.(model.Vector)
	if len(vector) >= 1 {
		rep.CurBalanceRegionCount = int(vector[0].Value)
	}

	result, warnings, err = v1api.Query(ctx,
		"pd_scheduler_event_count{type=\"balance-leader-scheduler\", name=\"schedule\"}", s.t.addTime)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	fmt.Printf("Result:\n%v\n", result)
	vector = result.(model.Vector)
	if len(vector) >= 1 {
		rep.PrevBalanceLeaderCount = int(vector[0].Value)
	}

	result, warnings, err = v1api.Query(ctx,
		"pd_scheduler_event_count{type=\"balance-region-scheduler\", name=\"schedule\"}", s.t.addTime)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	vector = result.(model.Vector)
	if len(vector) >= 1 {
		rep.PrevBalanceRegionCount = int(vector[0].Value)
	}
	fmt.Printf("percentage %d is %%", 10)
	bytes, err := json.Marshal(rep)
	return string(bytes), err
}

// lastReport is
func (s *scaleOut) mergeReport(lastReport, report string) (plainText string) {
	//todo @zeyuan
	last := &stats{}
	cur := &stats{}
	json.Unmarshal([]byte(lastReport), last)
	json.Unmarshal([]byte(report), cur)
	plainText = fmt.Sprintf(plainText+"Balance interval is %d, compared to origin by %.2f\n",
		last.Interval, float64((last.Interval-cur.Interval)/(cur.Interval+1)))
	plainText = fmt.Sprintf(plainText+"Prev Balance leader is %.2f, compared to origin by %.2f\n",
		float64(last.PrevBalanceLeaderCount), float64((last.PrevBalanceLeaderCount-cur.PrevBalanceLeaderCount)/(cur.PrevBalanceLeaderCount+1)))
	plainText = fmt.Sprintf(plainText+"Prev balance region is %.2f, compared to origin by %.2f\n",
		float64(last.PrevBalanceRegionCount), float64((last.PrevBalanceRegionCount-cur.PrevBalanceRegionCount)/(cur.PrevBalanceRegionCount+1)))
	plainText = fmt.Sprintf(plainText+"Cur balance leader is %.2f, compared to origin by %.2f\n",
		float64(last.PrevBalanceLeaderCount), float64((last.PrevBalanceLeaderCount-cur.PrevBalanceLeaderCount)/(cur.PrevBalanceLeaderCount+1)))
	plainText = fmt.Sprintf(plainText+"Cur balance region is %.2f, compared to origin by %.2f\n",
		float64(last.PrevBalanceRegionCount), float64((last.PrevBalanceRegionCount-cur.PrevBalanceRegionCount)/(cur.PrevBalanceRegionCount+1)))
	plainText = fmt.Sprintf(plainText+"Prev latency is %.2f, compared to origin by %.2f\n",
		last.PrevLatency, (last.PrevLatency-cur.PrevLatency)/(cur.PrevLatency+1))
	plainText = fmt.Sprintf(plainText+"Cur latency is %.2f, compared to origin by %.2f\n",
		last.CurLatency, (last.CurLatency-cur.CurLatency)/(cur.CurLatency+1))
	return
}
