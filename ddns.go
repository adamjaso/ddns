package main

import (
	"errors"
	"net"

	"golang.org/x/net/dns/dnsmessage"
)

// Answer parses a raw DNS query and returns a raw DNS response ready to write
// to a UDP socket. Only A and AAAA queries are supported.
//
// resolve is called with the queried hostname and qtype. Return nil to produce
// a NXDOMAIN response.
func Answer(query []byte, resolve func(name string, qtype dnsmessage.Type) (net.IP, error)) ([]byte, error) {
	var p dnsmessage.Parser
	hdr, err := p.Start(query)
	if err != nil {
		return nil, err
	}

	q, err := p.Question()
	if err != nil {
		return nil, err
	}

	switch q.Type {
	case dnsmessage.TypeA, dnsmessage.TypeAAAA:
	default:
		return notImpl(hdr.ID, q)
	}

	ip, err := resolve(q.Name.String(), q.Type)
	if err != nil {
		return nil, err
	}

	rhdr := dnsmessage.Header{
		ID:               hdr.ID,
		Response:         true,
		RecursionDesired: hdr.RecursionDesired,
		RCode:            dnsmessage.RCodeNameError,
	}
	if ip != nil {
		rhdr.RCode = dnsmessage.RCodeSuccess
	}

	b := dnsmessage.NewBuilder(nil, rhdr)
	b.EnableCompression()

	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(q); err != nil {
		return nil, err
	}

	if ip != nil {
		if err := b.StartAnswers(); err != nil {
			return nil, err
		}
		rh := dnsmessage.ResourceHeader{
			Name:  q.Name,
			Class: dnsmessage.ClassINET,
			TTL:   60,
		}
		switch q.Type {
		case dnsmessage.TypeA:
			v4 := ip.To4()
			if v4 == nil {
				return nil, errors.New("resolver returned non-IPv4 address for A query")
			}
			rh.Type = dnsmessage.TypeA
			var a [4]byte
			copy(a[:], v4)
			if err := b.AResource(rh, dnsmessage.AResource{A: a}); err != nil {
				return nil, err
			}
		case dnsmessage.TypeAAAA:
			v6 := ip.To16()
			if v6 == nil {
				return nil, errors.New("resolver returned non-IPv6 address for AAAA query")
			}
			rh.Type = dnsmessage.TypeAAAA
			var aaaa [16]byte
			copy(aaaa[:], v6)
			if err := b.AAAAResource(rh, dnsmessage.AAAAResource{AAAA: aaaa}); err != nil {
				return nil, err
			}
		}
	}

	return b.Finish()
}

func notImpl(id uint16, q dnsmessage.Question) ([]byte, error) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		ID:       id,
		Response: true,
		RCode:    dnsmessage.RCodeNotImplemented,
	})
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(q); err != nil {
		return nil, err
	}
	return b.Finish()
}
