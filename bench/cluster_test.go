package bench

import (
	"encoding/json"
	"net/http"
	"testing"

	. "github.com/pingcap/check"
)

func Test(t *testing.T) {
	TestingT(t)
}

type testCmdSuite struct{}

var _ = Suite(&testCmdSuite{})

func (s *testCmdSuite) TestCmd(c *C) {
	url := "http://172.16.5.110:8000/api/cluster/workload/30126/result"
	resp, err := doRequest(url, http.MethodGet)
	c.Assert(err, IsNil)

	reports := make([]WorkloadReport, 0)
	err = json.Unmarshal([]byte(resp), &reports)
	c.Assert(err, IsNil)
}
