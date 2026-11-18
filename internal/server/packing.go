package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"order_processor/internal/adapters/miaoshou/web"
	app "order_processor/internal/application/packing"
	"order_processor/internal/domain/packing"
	"order_processor/internal/store"
	"time"
)

func (g *browserGateway) packing(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.Exists(g.db, id) {
		send(w, 404, map[string]string{"error": "账户不存在"})
		return
	}
	var in packing.Request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil {
		send(w, 400, map[string]string{"error": "打包信息格式错误"})
		return
	}
	in.PackageID = r.PathValue("package")
	// Finish recording a write even if the employee closes their browser tab.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := g.submitPacking(ctx, id, in, true)
	if err != nil {
		code := 400
		if errors.Is(err, errPackingBusy) {
			code = 409
		}
		send(w, code, map[string]string{"error": err.Error()})
		return
	}
	send(w, 200, result)
}

var errPackingBusy = errors.New("账户正在操作浏览器或远程桌面，请结束后继续任务")

func (g *browserGateway) Submit(ctx context.Context, id string, in packing.Request) (packing.Result, error) {
	return g.submitPacking(ctx, id, in, false)
}

func (g *browserGateway) submitPacking(ctx context.Context, id string, in packing.Request, record bool) (packing.Result, error) {
	if !g.packingMu.TryLock() {
		return packing.Result{}, errPackingBusy
	}
	defer g.packingMu.Unlock()
	gate := g.gate(id)
	if !gate.TryLock() {
		return packing.Result{}, errPackingBusy
	}
	defer gate.Unlock()
	if gate.desktop {
		return packing.Result{}, errPackingBusy
	}
	repo := store.PackingRepository{DB: g.db}
	adapter := web.PackingSubmitter{Runtime: g.runtime, Client: &http.Client{Timeout: 45 * time.Second}, StorageKey: func(account string) string { return store.StorageKey(g.db, account) }}
	svc := app.Service{Repository: repo, Submitter: app.ShippingFirst{Repository: repo, Gateway: adapter, Next: adapter}}
	if record {
		repo := store.PackingJobs{DB: g.db}
		expanded, err := repo.IncludePackageGifts(ctx, id, packing.JobItem{Request: in})
		if err != nil {
			return packing.Result{}, err
		}
		in = expanded.Request
		svc.Recorder = repo
	}
	return svc.Submit(ctx, id, in)
}
