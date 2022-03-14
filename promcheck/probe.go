package promcheck

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

// HTTPClient represents http client
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

var defaultHTTPClient HTTPClient = newDefaultHTTPClient()

type prometheusProbe struct {
	client        HTTPClient
	delay         time.Duration
	prometheusUrl string
}

func newPrometheusProbe(delay time.Duration, prometheusUrl string, client HTTPClient) Probe {
	if client == nil {
		client = defaultHTTPClient
	}
	return &prometheusProbe{
		client:        client,
		delay:         delay,
		prometheusUrl: prometheusUrl,
	}
}

func (p *prometheusProbe) ProbeSelector(selector string) (uint64, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/query", p.prometheusUrl), nil)
	if err != nil {
		return 0, fmt.Errorf("query request failed: %s", err)
	}
	count := fmt.Sprintf("count(%s)", selector)
	q := req.URL.Query()
	q.Add("query", count)
	req.URL.RawQuery = q.Encode()
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("rule request failed: %s", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %s", err)
	}

	payload := struct {
		Status string
		Data   struct {
			Result []struct {
				Metric map[string]string
				Value  []interface{}
			}
		}
	}{}
	err = json.Unmarshal(b, &payload)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal json: %s", err)
	}

	if payload.Status != "success" {
		return 0, fmt.Errorf("status in rule response was not != success: was %s", payload.Status)
	}

	if len(payload.Data.Result) != 1 {
		return 0, nil
	}

	i, err := strconv.ParseUint(payload.Data.Result[0].Value[1].(string), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse int: %s", err)
	}

	time.Sleep(p.delay)

	return i, nil
}

func newDefaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          10,
			IdleConnTimeout:       60 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConnsPerHost:   runtime.GOMAXPROCS(0) + 1,
		},
	}
}
