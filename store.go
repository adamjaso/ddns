package main

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/net/dns/dnsmessage"
)

var store sync.Map // storeKey -> net.IP

type storeKey struct {
	qtype  dnsmessage.Type
	domain string
}

type cacheFile struct {
	A    map[string]string `json:"a"`
	AAAA map[string]string `json:"aaaa"`
}

func storeIP(domain string, ip net.IP) {
	if ip.To4() != nil {
		store.Store(storeKey{dnsmessage.TypeA, domain}, ip)
	} else {
		store.Store(storeKey{dnsmessage.TypeAAAA, domain}, ip)
	}
}

func resolve(name string, qtype dnsmessage.Type) (net.IP, error) {
	switch qtype {
	case dnsmessage.TypeA, dnsmessage.TypeAAAA:
		v, ok := store.Load(storeKey{qtype, name})
		if !ok {
			return nil, nil
		}
		return v.(net.IP), nil
	default:
		return nil, errors.New("unsupported query type")
	}
}

func loadCache(path string) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		log.Printf("cache read error: %v", err)
		return
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("cache parse error: %v", err)
		return
	}
	for d, s := range c.A {
		if ip := net.ParseIP(s); ip != nil {
			store.Store(storeKey{dnsmessage.TypeA, d}, ip)
		}
	}
	for d, s := range c.AAAA {
		if ip := net.ParseIP(s); ip != nil {
			store.Store(storeKey{dnsmessage.TypeAAAA, d}, ip)
		}
	}
	log.Printf("loaded cache from %s", path)
}

func writeCache(path string) error {
	c := cacheFile{
		A:    map[string]string{},
		AAAA: map[string]string{},
	}
	store.Range(func(k, v any) bool {
		key := k.(storeKey)
		ip := v.(net.IP).String()
		switch key.qtype {
		case dnsmessage.TypeA:
			c.A[key.domain] = ip
		case dnsmessage.TypeAAAA:
			c.AAAA[key.domain] = ip
		}
		return true
	})
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
