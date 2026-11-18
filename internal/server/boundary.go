package server

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// Local development accepts loopback and 192.168.0.0/16 LAN addresses.
// An explicit public origin keeps the server deployment restricted to that address.
func requestBoundary(next http.Handler, publicOrigin string) http.Handler {
	public, _ := url.Parse(publicOrigin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, e := net.SplitHostPort(r.Host)
		if e != nil {
			host = r.Host
		}
		loopback := host == "127.0.0.1" || host == "localhost" || host == "::1"
		ip, err := netip.ParseAddr(host)
		lan := err == nil && netip.MustParsePrefix("192.168.0.0/16").Contains(ip)
		allowed := loopback || lan
		if publicOrigin != "" {
			allowed = public != nil && r.Host == public.Host
		}
		if !allowed {
			http.Error(w, "访问地址未配置", 403)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/desktop/") {
			w.Header().Set("Cache-Control", "no-store")
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				good := e == nil && u.Scheme == "http" && u.Host == r.Host && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.User == nil
				if publicOrigin != "" {
					good = origin == publicOrigin
				} else if loopback && (origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173") {
					good = true
				}
				if !good {
					http.Error(w, "不允许跨站请求", 403)
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "不允许跨站请求", 403)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
