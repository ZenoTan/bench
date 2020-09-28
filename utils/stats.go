package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/go-echarts/go-echarts/charts"
)

type CompareStats interface {
	Init(last, cur string)
	CollectFrom(fileName string)
	RenderTo(fileName string)
	Report() string
}

var scaleOutStatsOrder = []string{
	"BalanceInterval",
	"PrevBalanceLeaderCount",
	"PrevBalanceRegionCount",
	"CurBalanceLeaderCount",
	"CurBalanceRegionCount",
	"PrevLatency",
	"CurLatency",
	"PrevCompactionRate",
	"CurCompactionRate",
	"PrevApplyLog",
	"CurApplyLog",
	"PrevDbMutex",
	"CurDbMutex",
}

type ScaleOutOnce struct {
	BalanceInterval        int     `json:"BalanceInterval"`
	PrevBalanceLeaderCount int     `json:"PrevBalanceLeaderCount"`
	PrevBalanceRegionCount int     `json:"PrevBalanceRegionCount"`
	CurBalanceLeaderCount  int     `json:"CurBalanceLeaderCount"`
	CurBalanceRegionCount  int     `json:"CurBalanceRegionCount"`
	PrevLatency            float64 `json:"PrevLatency"`
	CurLatency             float64 `json:"CurLatency"`
	PrevCompactionRate     float64 `json:"PrevCompactionRate"`
	CurCompactionRate      float64 `json:"CurCompactionRate"`
	PrevApplyLog           float64 `json:"PrevApplyLog"`
	CurApplyLog            float64 `json:"CurApplyLog"`
	PrevDbMutex            float64 `json:"PrevDbMutex"`
	CurDbMutex             float64 `json:"CurDbMutex"`
}

type scaleOutStats struct {
	CompareStats
	statsMap *map[string][2]float64
}

func (s *scaleOutStats) Init(last, cur string) {
	if last == "" || cur == "" {
		return
	}
	var lastStats, curStats ScaleOutOnce
	json.Unmarshal([]byte(last), &lastStats)
	json.Unmarshal([]byte(cur), &curStats)
	m := make(map[string][2]float64)
	t := reflect.TypeOf(lastStats)
	v1 := reflect.ValueOf(lastStats)
	v2 := reflect.ValueOf(curStats)
	for i := 0; i < v1.NumField(); i++ {
		var val1, val2 float64
		if t.Field(i).Type.String() == "int" {
			val1 = float64(v1.Field(i).Int())
			val2 = float64(v2.Field(i).Int())
		} else {
			val1 = v1.Field(i).Float()
			val2 = v2.Field(i).Float()
		}
		m[t.Field(i).Name] = [2]float64{val1, val2}
	}
	s.statsMap = &m
}

func (s *scaleOutStats) CollectFrom(fileName string) {
	// todo: load from file
	return
}

func (s *scaleOutStats) RenderTo(fileName string) {
	m := *s.statsMap
	var lastData, curData []float64
	for _, stat := range scaleOutStatsOrder {
		mid := (m[stat][0] + m[stat][1]) / 2
		lastData = append(lastData, m[stat][0] / (mid + 1e-6))
		curData = append(curData, m[stat][1] / (mid + 1e-6))
	}
	var xAxis []string
	for i := range scaleOutStatsOrder {
		xAxis = append(xAxis, "p" + strconv.Itoa(i))
	}
	bar := charts.NewBar()
	bar.SetGlobalOptions(charts.TitleOpts{Title: "scale out stats"}, charts.ToolboxOpts{Show: true})
	bar.AddXAxis(xAxis).
		AddYAxis("last", lastData).
		AddYAxis("cur", curData)
	f, _ := os.Create(fileName)
	bar.Render(f)
}

func (s *scaleOutStats) Report() string {
	m := *s.statsMap
	text := "Label:\n"
	for i, s := range scaleOutStatsOrder {
		text += "p" + strconv.Itoa(i) + ": " + s + "\n"
		text += fmt.Sprintf("standard(last, red) is %.6f\n", m[s][0])
	}
	return text
}
