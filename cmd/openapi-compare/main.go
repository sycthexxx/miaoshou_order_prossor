// openapi-compare is an explicit, read-only diagnostic. It is not started by
// the application. Credentials are read without echo from the user's terminal.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	api "order_processor/internal/adapters/miaoshou/openapi"
	"order_processor/internal/diagnostics/openapicompare"
)

func credentials() (api.Credentials, error) {
	var c api.Credentials
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return c, errors.New("an interactive Mac/Linux terminal is required; credentials are never accepted as command arguments")
	}
	defer tty.Close()
	stty := func(args ...string) error { cmd := exec.Command("stty", args...); cmd.Stdin = tty; return cmd.Run() }
	if err = stty("-echo"); err != nil {
		return c, errors.New("cannot disable terminal echo")
	}
	defer stty("echo")
	interrupts := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)
	defer close(done)
	go func() {
		select {
		case <-interrupts:
			_ = stty("echo")
			os.Exit(130)
		case <-done:
		}
	}()
	reader := bufio.NewReader(tty)
	read := func(label string) (string, error) {
		fmt.Fprint(tty, label)
		s, e := reader.ReadString('\n')
		fmt.Fprintln(tty)
		return strings.TrimSpace(s), e
	}
	if c.AppKey, err = read("AppKey（不回显）: "); err != nil {
		return c, errors.New("credential input cancelled")
	}
	if c.AppSecret, err = read("AppSecret（不回显）: "); err != nil {
		return c, errors.New("credential input cancelled")
	}
	if c.SubAccountID, err = read("子账户ID（可空，不回显）: "); err != nil {
		return c, errors.New("credential input cancelled")
	}
	return c, nil
}
func run() error {
	dir := flag.String("raw-dir", "", "explicit single-account raw archive directory (required)")
	limit := flag.Int("limit", 3, "sample count (1..5)")
	cases := flag.Bool("cases", false, "select evidenced hasPurchase, purchase change and partial refund cases (at most 3 unique packages)")
	dry := flag.Bool("dry-run", false, "only inspect local sample; no credentials or network")
	responseDir := flag.String("response-dir", "", "save complete official response JSON to this private local directory")
	inspect := flag.Bool("inspect-states", false, "inventory full response paths and candidate state values, including embedded JSON")
	packageTab := flag.String("package-tab", "", "official appPackageTab filter; verify accepted values against the account")
	packageStatus := flag.String("package-status", "", "official appPackageStatus filter")
	flag.Parse()
	if *dir == "" {
		return errors.New("--raw-dir must explicitly select one account directory")
	}
	var sample compare.Sample
	var evidence []compare.CaseEvidence
	var err error
	if *cases {
		sample, evidence, err = compare.ReadCases(*dir)
	} else {
		sample, err = compare.ReadSample(*dir, *limit)
	}
	if err != nil {
		return err
	}
	if *cases {
		compare.Print(os.Stdout, map[string]any{"cases": evidence, "note": "类别证据为历史采样时刻；实际对照使用该包裹最近本地版本。找不到部分退款不以取消/全退款替代。"})
	}
	if len(sample.Packages) == 0 {
		return errors.New("no evidenced sample available; no API request sent")
	}
	locals := []compare.Object{}
	ids := []string{}
	for _, raw := range sample.Packages {
		p, e := compare.Decode(raw)
		if e != nil {
			return errors.New("invalid sample JSON")
		}
		locals = append(locals, p)
		ids = append(ids, compare.String(p["opOrderPackageId"]))
	}
	compare.Print(os.Stdout, map[string]any{"localAccount": sample.Account, "sourceCapturedAt": sample.CapturedAt, "samplePackageIds": ids, "sampleCount": len(ids), "dryRun": *dry, "note": "只比较选定本地账户样本；凭证权限需对应同一账户。源快照与官方当前状态时间不同，未命中不代表不同商品。"})
	if *dry {
		for _, p := range locals {
			if *inspect {
				raw, _ := json.Marshal(p)
				fields, e := compare.Inspect(raw)
				if e != nil {
					return e
				}
				compare.Print(os.Stdout, map[string]any{"channel": "local", "packageId": compare.String(p["opOrderPackageId"]), "fieldInventory": fields})
			}
			compare.Print(os.Stdout, map[string]any{"packageId": compare.String(p["opOrderPackageId"]), "localItemCoverage": compare.LocalItemCoverage(p), "note": "仅本地结构验证，未获取官方数据，不能推断匹配结果"})
		}
		return nil
	}
	creds, err := credentials()
	if err != nil {
		return err
	}
	options := api.Options{}
	if *inspect || *responseDir != "" {
		options.ObserveResponse = func(path string, request, raw json.RawMessage) error {
			if *responseDir != "" {
				if err := saveResponse(*responseDir, path, request, raw, *packageTab, *packageStatus); err != nil {
					return err
				}
			}
			if !*inspect {
				return nil
			}
			fields, e := compare.Inspect(raw)
			if e == nil {
				compare.Print(os.Stdout, map[string]any{"channel": "responseInventory", "endpoint": path, "observedAt": time.Now().UTC().Format(time.RFC3339), "appPackageTab": *packageTab, "appPackageStatus": *packageStatus, "fieldInventory": fields})
			}
			return e
		}
	}
	client, err := api.New(creds, options)
	if err != nil {
		return err
	}
	creds = api.Credentials{}
	// A small bounded batch per platform; no retries or automatic page sweep.
	groups := map[string][]compare.Object{}
	platforms := []string{}
	for _, p := range locals {
		platform := compare.String(p["platform"])
		if _, ok := groups[platform]; !ok {
			platforms = append(platforms, platform)
		}
		groups[platform] = append(groups[platform], p)
	}
	listFound, detailFound := 0, 0
	for _, platform := range platforms {
		orders := []string{}
		seen := map[string]bool{}
		for _, p := range groups[platform] {
			s := compare.String(compare.Value(p, "orderInfo.platformOrderSn"))
			if !seen[s] {
				orders = append(orders, s)
				seen[s] = true
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		list, e := client.ListPackages(ctx, api.ListRequest{Page: 1, PageSize: 200, Platform: platform, PlatformOrderSns: strings.Join(orders, ","), AppPackageTab: *packageTab, AppPackageStatus: *packageStatus})
		cancel()
		if e != nil {
			return e
		}
		byID := map[string]compare.Object{}
		for _, raw := range list.OrderPackageList {
			p, e := compare.Decode(raw)
			if e == nil {
				byID[compare.String(p["opOrderPackageId"])] = p
			}
		}
		if list.Total > int64(len(list.OrderPackageList)) {
			compare.Print(os.Stdout, map[string]any{"channel": "list", "incomplete": true, "total": list.Total, "received": len(list.OrderPackageList), "note": "仅检查首个200条页面，未命中不作缺失结论"})
		}
		for _, p := range groups[platform] {
			id := compare.String(p["opOrderPackageId"])
			remote := byID[id]
			if remote != nil {
				listFound++
			}
			compare.Print(os.Stdout, compare.Compare(p, remote, "list"))
		}
		time.Sleep(time.Second)
	}
	for i, p := range locals {
		id, e := strconv.ParseInt(compare.String(p["opOrderPackageId"]), 10, 64)
		if e != nil || id <= 0 {
			return errors.New("sample has invalid package ID")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		raw, e := client.GetPackage(ctx, id)
		cancel()
		if e != nil {
			compare.Print(os.Stdout, map[string]any{"channel": "detail", "packageId": strconv.FormatInt(id, 10), "error": e.Error()})
		} else {
			remote, e := compare.Decode(raw)
			if e != nil {
				return errors.New("invalid detail JSON")
			}
			if compare.String(remote["opOrderPackageId"]) != strconv.FormatInt(id, 10) {
				compare.Print(os.Stdout, map[string]any{"channel": "detail", "packageId": strconv.FormatInt(id, 10), "error": "response package ID missing or different; not compared"})
			} else {
				detailFound++
				compare.Print(os.Stdout, compare.Compare(p, remote, "detail"))
			}
		}
		if i+1 < len(locals) {
			time.Sleep(time.Second)
		}
	}
	return compare.Print(os.Stdout, map[string]any{"sampleCount": len(locals), "listMatched": listFound, "detailMatched": detailFound, "note": "结果仅限样本与查询时刻；不证明待处理分组、退款或货源关联等价。"})
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// The complete response JSON is retained in a local envelope with query context.
func saveResponse(dir, endpoint string, request, raw json.RawMessage, tab, status string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".response-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	envelope := struct {
		CapturedAt       string          `json:"capturedAt"`
		Endpoint         string          `json:"endpoint"`
		AppPackageTab    string          `json:"appPackageTab,omitempty"`
		AppPackageStatus string          `json:"appPackageStatus,omitempty"`
		Request          json.RawMessage `json:"request"`
		Response         json.RawMessage `json:"response"`
	}{time.Now().UTC().Format(time.RFC3339Nano), endpoint, tab, status, request, raw}
	err = json.NewEncoder(f).Encode(envelope)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	destination := filepath.Join(dir, time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+filepath.Base(endpoint)+"-"+filepath.Base(temp)[len(".response-"):]+".json")
	if err := os.Rename(temp, destination); err != nil {
		return err
	}
	return compare.Print(os.Stdout, map[string]any{"savedResponse": destination, "endpoint": endpoint})
}
