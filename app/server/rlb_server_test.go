package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/rlb/app/config"
	"github.com/umputun/rlb/app/picker"
)

func TestDoJump(t *testing.T) {

	srv := NewRLBServer(newMockPicker(), "error msg", "", 0, "v1")
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()
	defer srv.Shutdown()

	r, err := hit(hitReq{"svc1", "/file123.mp3", ts.URL})
	assert.NoError(t, err)
	assert.Equal(t, "http://srv1.com/file123.mp3", r)

	r, err = hit(hitReq{"svc1", "/file1234.mp3", ts.URL})
	assert.NoError(t, err)
	assert.Equal(t, "http://srv2.com/file1234.mp3", r)

	r, err = hit(hitReq{"svc2", "/file12345.mp3", ts.URL})
	assert.NoError(t, err)
	assert.Equal(t, "http://srv1.com/file12345.mp3", r)
}

func TestSubmitStats(t *testing.T) {

	statsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/stat", r.URL.Path)
		lrec := LogRecord{}
		err := json.NewDecoder(r.Body).Decode(&lrec)
		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1", lrec.FromIP)
		assert.Equal(t, "srv1.com", lrec.DestHost)
		assert.Equal(t, "svc1/file123.mp3", lrec.Fname)
	}))
	defer statsSrv.Close()

	srv := NewRLBServer(newMockPicker(), "error msg", statsSrv.URL+"/stat", 0, "v1")
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()
	defer srv.Shutdown()

	r, err := hit(hitReq{"svc1", "/file123.mp3", ts.URL})
	assert.NoError(t, err)
	assert.Equal(t, "http://srv1.com/file123.mp3", r)

	time.Sleep(100 * time.Millisecond)
}

type hitReq struct {
	svc      string
	resource string
	url      string
}

func hit(r hitReq) (location string, err error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	url := fmt.Sprintf("%s/api/v1/jump/%s?url=%s", r.url, r.svc, r.resource)
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 302 {
		return "", errors.Errorf("wrong status code %d", resp.StatusCode)
	}
	return resp.Header.Get("Location"), nil
}

type mockPicker struct {
	nodes map[string][]picker.Node
	ids   map[string]int
}

func newMockPicker() *mockPicker {
	return &mockPicker{
		ids: map[string]int{},
		nodes: map[string][]picker.Node{
			"svc1": []picker.Node{
				{Node: config.Node{Server: "http://srv1.com"}},
				{Node: config.Node{Server: "http://srv2.com"}},
			},
			"svc2": []picker.Node{
				{Node: config.Node{Server: "http://srv1.com"}},
				{Node: config.Node{Server: "http://srv2.com"}},
				{Node: config.Node{Server: "http://srv3.com"}},
			},
		},
	}
}

func (m *mockPicker) Pick(svc string, resource string) (resURL string, node picker.Node, err error) {
	svcNodes, ok := m.nodes[svc]
	if !ok {
		return "", node, errors.Errorf("no such service %s", svc)
	}
	id := m.ids[svc]
	m.ids[svc] = id + 1
	if m.ids[svc] >= len(svcNodes) {
		m.ids[svc] = 0
	}
	return svcNodes[id].Server + resource, svcNodes[id], nil
}

func (m *mockPicker) Nodes() map[string][]picker.Node {
	return m.nodes
}
