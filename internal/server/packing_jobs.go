package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	app "order_processor/internal/application/packing"
	"order_processor/internal/domain/packing"
	"order_processor/internal/store"
	"strconv"
	"strings"
)

func packingJobRoutes(ctx context.Context, mux *http.ServeMux, g *browserGateway) {
	repo := store.PackingJobs{DB: g.db}
	if ctx != nil {
		go func() {
			if err := (app.Queue{Repository: repo, Submitter: g}).Run(ctx); err != nil {
				log.Printf("打包任务执行停止：%v", err)
			}
		}()
	}
	mux.HandleFunc("GET /api/accounts/{id}/packing-jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := repo.List(r.Context(), r.PathValue("id"))
		if err != nil {
			send(w, 500, map[string]string{"error": "读取任务失败"})
			return
		}
		send(w, 200, jobs)
	})
	mux.HandleFunc("GET /api/accounts/{id}/packing-jobs/{job}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("job"), 10, 64)
		job, err := repo.Detail(r.Context(), r.PathValue("id"), id)
		if err != nil {
			send(w, 404, map[string]string{"error": "任务不存在"})
			return
		}
		send(w, 200, job)
	})
	mux.HandleFunc("POST /api/accounts/{id}/packing-jobs", func(w http.ResponseWriter, r *http.Request) {
		account := r.PathValue("id")
		if !store.Exists(g.db, account) {
			send(w, 404, map[string]string{"error": "账户不存在"})
			return
		}
		var in packing.BatchInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil {
			send(w, 400, map[string]string{"error": "任务信息格式错误"})
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" {
			in.Name = "货源代打包"
		}
		candidates, err := repo.Candidates(account, in.SourceKey, in.Gifts)
		if err != nil {
			send(w, 400, map[string]string{"error": err.Error()})
			return
		}
		items, err := packing.PrepareBatch(in, candidates)
		if err != nil {
			send(w, 400, map[string]string{"error": err.Error()})
			return
		}
		for index, item := range items {
			items[index], err = repo.IncludePackageGifts(r.Context(), account, item)
			if err != nil {
				send(w, 400, map[string]string{"error": err.Error()})
				return
			}
		}
		id, err := repo.Create(r.Context(), account, in, items)
		if err != nil {
			send(w, 409, map[string]string{"error": err.Error()})
			return
		}
		send(w, 201, map[string]any{"id": id, "packages": len(items)})
	})
	mux.HandleFunc("GET /api/accounts/{id}/packing-jobs/{job}/rerun-preview", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("job"), 10, 64)
		job, err := repo.RerunPreview(r.Context(), r.PathValue("id"), id)
		if err != nil {
			send(w, 400, map[string]string{"error": err.Error()})
			return
		}
		send(w, 200, job)
	})
	mux.HandleFunc("POST /api/accounts/{id}/packing-jobs/{job}/rerun", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("job"), 10, 64)
		var in store.PackingRerunInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil {
			send(w, 400, map[string]string{"error": "任务信息格式错误"})
			return
		}
		created, err := repo.Rerun(r.Context(), r.PathValue("id"), id, in)
		if err != nil {
			send(w, 409, map[string]string{"error": err.Error()})
			return
		}
		send(w, 201, map[string]any{"id": created})
	})
	mux.HandleFunc("POST /api/accounts/{id}/packing-jobs/{job}/{action}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("job"), 10, 64)
		var in struct {
			ItemID int64 `json:"itemId"`
		}
		if r.ContentLength != 0 && json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
			send(w, 400, map[string]string{"error": "操作格式错误"})
			return
		}
		if err := repo.Action(r.Context(), r.PathValue("id"), id, r.PathValue("action"), in.ItemID); err != nil {
			send(w, 409, map[string]string{"error": err.Error()})
			return
		}
		send(w, 200, map[string]bool{"ok": true})
	})
}
