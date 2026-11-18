package miaoshouopenapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Capture owns API response archival. The application only sees Source.Fetch.
type Capture struct {
	Source             Source
	StartedAt, Archive string
	directory          string
}

func NewCapture(credentials Credentials, root, accountKey string) (Capture, error) {
	started := time.Now().UTC()
	capture := Capture{StartedAt: started.Format("2006-01-02T15:04:05.000Z"), Archive: filepath.Join("openapi", accountKey, started.Format("20060102-150405.000000000"))}
	capture.directory = filepath.Join(root, capture.Archive)
	seq := 0
	client, err := New(credentials, Options{ObserveResponse: func(endpoint string, request, response json.RawMessage) error {
		seq++
		raw, err := json.MarshalIndent(map[string]any{"capturedAt": time.Now().UTC().Format(time.RFC3339Nano), "endpoint": endpoint, "request": request, "response": response}, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(capture.directory, fmt.Sprintf("%04d.json", seq)), raw, 0600); err != nil {
			return fmt.Errorf("完整响应保存失败")
		}
		return nil
	}})
	if err != nil {
		return Capture{}, err
	}
	if err = os.MkdirAll(capture.directory, 0700); err != nil {
		return Capture{}, fmt.Errorf("无法创建响应存档目录")
	}
	capture.Source = Source{Client: client}
	return capture, nil
}
func (c Capture) SaveResult(result any) error {
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.directory, "result.json"), raw, 0600)
}
