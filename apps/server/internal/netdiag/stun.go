// Package netdiag diagnostica a rede do servidor: interfaces e endereços,
// IP público via STUN (somente descoberta — nunca relay de mídia), CGNAT,
// IPv6, portas e firewall, com orientações compreensíveis ao administrador.
package netdiag

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

const stunMagicCookie = 0x2112A442

// STUNBinding envia um Binding Request (RFC 5389) e retorna o endereço
// público mapeado (XOR-MAPPED-ADDRESS ou MAPPED-ADDRESS).
func STUNBinding(ctx context.Context, network, server string) (net.IP, int, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, network, server)
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	} else {
		conn.SetDeadline(time.Now().Add(3 * time.Second))
	}
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req[0:], 0x0001) // Binding Request
	binary.BigEndian.PutUint16(req[2:], 0)
	binary.BigEndian.PutUint32(req[4:], stunMagicCookie)
	txID := req[8:20]
	if _, err := rand.Read(txID); err != nil {
		return nil, 0, err
	}
	if _, err := conn.Write(req); err != nil {
		return nil, 0, err
	}
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, 0, err
	}
	return parseBindingResponse(buf[:n], txID)
}

func parseBindingResponse(b, txID []byte) (net.IP, int, error) {
	if len(b) < 20 || binary.BigEndian.Uint16(b[0:]) != 0x0101 {
		return nil, 0, errors.New("resposta STUN inválida")
	}
	if binary.BigEndian.Uint32(b[4:]) != stunMagicCookie || string(b[8:20]) != string(txID) {
		return nil, 0, errors.New("transação STUN não confere")
	}
	msgLen := int(binary.BigEndian.Uint16(b[2:]))
	if 20+msgLen > len(b) {
		return nil, 0, errors.New("resposta STUN truncada")
	}
	attrs := b[20 : 20+msgLen]
	var mapped net.IP
	var mappedPort int
	for len(attrs) >= 4 {
		typ := binary.BigEndian.Uint16(attrs[0:])
		l := int(binary.BigEndian.Uint16(attrs[2:]))
		if 4+l > len(attrs) {
			break
		}
		v := attrs[4 : 4+l]
		switch typ {
		case 0x0020: // XOR-MAPPED-ADDRESS
			if ip, port, err := decodeAddr(v, true, b[4:20]); err == nil {
				return ip, port, nil
			}
		case 0x0001: // MAPPED-ADDRESS
			if ip, port, err := decodeAddr(v, false, nil); err == nil {
				mapped, mappedPort = ip, port
			}
		}
		pad := (4 - l%4) % 4
		attrs = attrs[4+l+pad:]
	}
	if mapped != nil {
		return mapped, mappedPort, nil
	}
	return nil, 0, errors.New("resposta STUN sem endereço mapeado")
}

func decodeAddr(v []byte, xor bool, cookieAndTx []byte) (net.IP, int, error) {
	if len(v) < 8 {
		return nil, 0, errors.New("atributo curto")
	}
	family := v[1]
	port := int(binary.BigEndian.Uint16(v[2:]))
	if xor {
		port ^= stunMagicCookie >> 16
	}
	var ip net.IP
	switch family {
	case 0x01:
		ip = make(net.IP, 4)
		copy(ip, v[4:8])
	case 0x02:
		if len(v) < 20 {
			return nil, 0, errors.New("IPv6 curto")
		}
		ip = make(net.IP, 16)
		copy(ip, v[4:20])
	default:
		return nil, 0, fmt.Errorf("família %d desconhecida", family)
	}
	if xor {
		for i := range ip {
			ip[i] ^= cookieAndTx[i]
		}
	}
	return ip, port, nil
}
