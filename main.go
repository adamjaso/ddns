package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

var (
	allowedDomains = map[string]struct{}{}
	authUserHash   string // "username:sha256hex(password)"
	noTrustProxy   bool
	cacheFilePath  string
	debug          bool
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "-genhash" {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Fprint(os.Stderr, "Username: ")
		scanner.Scan()
		username := scanner.Text()
		fmt.Fprint(os.Stderr, "Password: ")
		scanner.Scan()
		password := scanner.Text()
		sum := sha256.Sum256([]byte(password))
		fmt.Printf("BASIC_AUTH_USER_HASH=%s:%s\n", username, hex.EncodeToString(sum[:]))
		return
	}

	noTrustProxy = os.Getenv("NO_TRUST_PROXY") == "1"
	debug = os.Getenv("DEBUG") == "1"

	httpAddr := os.Getenv("HTTP_LISTEN")
	if httpAddr == "" {
		httpAddr = "127.0.0.1:50505"
	}
	dnsAddr := os.Getenv("DNS_LISTEN")
	if dnsAddr == "" {
		dnsAddr = ":53"
	}

	authUserHash = os.Getenv("BASIC_AUTH_USER_HASH")
	if authUserHash == "" {
		log.Fatal("BASIC_AUTH_USER_HASH is required (format: username:sha256hexofpassword)")
	}

	domains := os.Getenv("DOMAINS")
	if domains == "" {
		log.Fatal("DOMAINS is required (comma-separated list of domain names)")
	}
	for _, d := range strings.Split(domains, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			if !strings.HasSuffix(d, ".") {
				d += "."
			}
			allowedDomains[strings.ToLower(d)] = struct{}{}
		}
	}

	cacheFilePath = os.Getenv("CACHE_FILE")
	if cacheFilePath != "" {
		loadCache(cacheFilePath)
	}

	log.Printf("HTTP listening on %s", httpAddr)
	log.Printf("DNS  listening on %s", dnsAddr)

	go serveDNS(dnsAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleUpdate)
	log.Fatal(http.ListenAndServe(httpAddr, mux))
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if !checkAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="ddns"`)
		log.Printf("401 unauthorized from %s", r.RemoteAddr)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ip := clientIP(r)
	if ip == nil {
		http.Error(w, "could not determine remote IP", http.StatusBadRequest)
		return
	}

	requested := r.URL.Query()["domain"]
	if len(requested) == 0 {
		http.Error(w, "at least one 'domain' query parameter is required", http.StatusBadRequest)
		return
	}

	var updated []string
	for _, d := range requested {
		d = strings.ToLower(strings.TrimSpace(d))
		if !strings.HasSuffix(d, ".") {
			d += "."
		}
		if _, ok := allowedDomains[d]; !ok {
			log.Printf("403 forbidden: %s requested disallowed domain %q", ip, d)
			http.Error(w, fmt.Sprintf("domain %q is not in the allowed list", d), http.StatusForbidden)
			return
		}
		changed := storeIP(d, ip)
		if debug || changed {
			log.Printf("updated %s -> %s", d, ip)
		}
		updated = append(updated, d)
	}

	if cacheFilePath != "" {
		if err := writeCache(cacheFilePath); err != nil {
			log.Printf("cache write error: %v", err)
		}
	}
	fmt.Fprintf(w, "ok: %s -> %s\n", strings.Join(updated, ", "), ip)
}

func clientIP(r *http.Request) net.IP {
	if !noTrustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if ip := net.ParseIP(strings.TrimSpace(strings.SplitN(fwd, ",", 2)[0])); ip != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}

func checkAuth(r *http.Request) bool {
	username, password, ok := r.BasicAuth()
	if !ok {
		return false
	}
	sum := sha256.Sum256([]byte(password))
	candidate := username + ":" + hex.EncodeToString(sum[:])
	return candidate == authUserHash
}

func serveDNS(addr string) {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Fatalf("DNS listen: %v", err)
	}
	defer conn.Close()

	buf := make([]byte, 512)
	for {
		n, src, err := conn.ReadFrom(buf)
		if err != nil {
			log.Printf("DNS read error: %v", err)
			continue
		}

		resp, err := Answer(buf[:n], func(name string, qtype dnsmessage.Type) (net.IP, error) {
			ip, err := resolve(name, qtype)
			if ip != nil {
				log.Printf("DNS query: %s %s from %s -> %s", qtype, name, src, ip)
			}
			return ip, err
		})
		if err != nil {
			log.Printf("DNS answer error: %v", err)
			continue
		}
		if _, err := conn.WriteTo(resp, src); err != nil {
			log.Printf("DNS write error: %v", err)
		}
	}
}
