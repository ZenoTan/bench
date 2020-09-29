package utils

import (
	"encoding/json"
	"strings"
	"testing"

	. "github.com/pingcap/check"
)

func Test(t *testing.T) {
	TestingT(t)
}

type testStatsSuite struct{}

var _ = Suite(&testStatsSuite{})

func (s *testStatsSuite) TestScaleOutStats(c *C) {
	var prev, cur ScaleOutOnce
	prev.BalanceInterval = 120
	cur.BalanceInterval = 100
	prev.PrevLatency = 0.735
	cur.PrevLatency = 0.893
	bytes1, _ := json.Marshal(prev)
	bytes2, _ := json.Marshal(cur)
	stats := &scaleOutStats{}
	stats.Init(string(bytes1), string(bytes2))
	stats.RenderTo("test_scale.html")
	report := stats.Report()
	c.Assert(strings.Contains(report, "p0: BalanceInterval"), Equals, true)
	c.Assert(strings.Contains(report, "standard(last, red) is"), Equals, true)
}
