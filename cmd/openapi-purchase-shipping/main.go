// Standalone experiment for api-479599777. Never launched by the web application.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	api "order_processor/internal/adapters/miaoshou/openapi"
	"order_processor/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("openapi-purchase-shipping", flag.ContinueOnError)
	flags.SetOutput(out)
	input := flags.String("request", "", "采购信息 JSON 文件（必填）")
	account := flags.String("account", "", "本地账户 ID，提交时读取其当前 API 凭证")
	database := flags.String("db", "data/orders.db", "本地数据库，仅只读访问")
	responseDir := flags.String("response-dir", "data/openapi-purchase-shipping", "请求与完整响应存档目录")
	submit := flags.Bool("submit", false, "实际发送一次更新请求；默认只预览，不联网")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return errors.New("请指定 --request JSON 文件")
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	var request api.UpdatePurchaseShippingRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("采购信息 JSON 格式错误: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("请求文件只能包含一个 JSON 对象")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	preview, _ := json.MarshalIndent(request, "", "  ")
	fmt.Fprintf(out, "POST %s%s\n%s\n", api.BaseURL, api.UpdatePurchaseShippingPath, preview)
	if !*submit {
		fmt.Fprintln(out, "仅预览，未读取凭证、未发送请求。实际提交需添加 --account 和 --submit。")
		return nil
	}
	if *account == "" {
		return errors.New("实际提交必须指定 --account")
	}
	abs, err := filepath.Abs(*database)
	if err != nil {
		return err
	}
	dsn := (&url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro&_busy_timeout=5000"}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	credential, err := store.CurrentAPICredential(db, *account)
	if err != nil {
		return fmt.Errorf("读取当前 API 凭证失败: %w", err)
	}
	client, err := api.New(api.Credentials{AppKey: credential.APIID, AppSecret: credential.Key, SubAccountID: credential.SubAccountID}, api.Options{})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "账户 %s，当前凭证：%s\n", *account, credential.Name)
	dir := filepath.Join(*responseDir, time.Now().UTC().Format("20060102-150405.000000000"))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	meta, _ := json.MarshalIndent(map[string]any{"endpoint": api.UpdatePurchaseShippingPath, "accountId": *account, "credentialId": credential.ID, "credentialName": credential.Name, "startedAt": time.Now().UTC()}, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "attempt.json"), meta, 0600); err != nil {
		return err
	}
	fmt.Fprintln(out, "存档目录：", dir)
	return submitAndSave(ctx, client, request, dir, out)
}

func submitAndSave(ctx context.Context, client *api.Client, request api.UpdatePurchaseShippingRequest, dir string, out io.Writer) error {
	body, _ := json.Marshal(request)
	if err := os.WriteFile(filepath.Join(dir, "request.json"), body, 0600); err != nil {
		return err
	}
	response, callErr := client.UpdatePurchaseShipping(ctx, request)
	responseFile := ""
	if response.Body != nil {
		responseFile = "response.json"
		if !json.Valid(response.Body) {
			responseFile = "response.txt"
		}
		if err := os.WriteFile(filepath.Join(dir, responseFile), response.Body, 0600); err != nil {
			return fmt.Errorf("请求已发送，但响应存档失败；请核对远端结果，不要直接重试: %w", err)
		}
	}
	result := map[string]any{"finishedAt": time.Now().UTC(), "httpStatus": response.Status, "responseFile": responseFile, "apiReportedSuccess": callErr == nil}
	if callErr != nil {
		result["error"] = callErr.Error()
	}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "result.json"), encoded, 0600); err != nil {
		return fmt.Errorf("请求已发送，但结果记录失败；请核对远端结果，不要直接重试: %w", err)
	}
	if callErr != nil {
		return callErr
	}
	fmt.Fprintln(out, "API 返回成功，完整响应已保存；是否关联目标订单及改变分组，需要另行回查。")
	return nil
}
