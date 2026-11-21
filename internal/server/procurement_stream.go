package server

import "net/http"

func (c *procurementCache) stream(auth *authentication) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account, gifts := r.PathValue("id"), r.URL.Query().Get("gifts") == "1"
		updates, unsubscribe, err := c.subscribe(account, gifts)
		if err != nil {
			send(w, 500, map[string]string{"error": "采购汇总读取失败"})
			return
		}
		defer unsubscribe()
		serveAccountStream(w, r, auth, updates, func(summary map[string]any, write func(any) error) error {
			return write(map[string]any{"type": "procurement_snapshot", "account": account, "gifts": gifts, "summary": summary})
		})
	}
}
