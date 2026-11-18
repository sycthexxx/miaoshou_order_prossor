package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCompleteResponse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "responses")
	raw := json.RawMessage(`{"result":"success","unknown":{"state":"new"},"data":{"orderPackageInfo":{"id":9007199254740993,"extInfo":"{\"status\":2}"}}}`)
	request := json.RawMessage(`{"opOrderPackageId":9007199254740993}`)
	if err := saveResponse(dir, "/get_package_info", request, raw, "", ""); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Response json.RawMessage
		Request  json.RawMessage
	}
	if err = json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if string(saved.Response) != string(raw) || string(saved.Request) != string(request) {
		t.Fatal("lost response fields or request context")
	}
	info, _ := os.Stat(files[0])
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if strings.Contains(string(b), "x-app-key") {
		t.Fatal("credentials stored")
	}
}
