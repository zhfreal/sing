package ntp

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"time"

	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
)

func Exchange(ctx context.Context, dialer N.Dialer, serverAddress M.Socksaddr) (*Response, error) {
	conn, err := dialer.DialContext(ctx, N.NetworkUDP, serverAddress)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(defaultTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	conn.SetDeadline(deadline)

	var request msg
	request.setMode(client)
	request.setVersion(defaultNtpVersion)
	request.setLeap(LeapNotInSync)

	bits := make([]byte, 8)
	_, err = rand.Read(bits)
	var xmitTime time.Time
	if err == nil {
		request.TransmitTime = ntpTime(binary.BigEndian.Uint64(bits))
		xmitTime = time.Now()
	} else {
		xmitTime = time.Now()
		request.TransmitTime = toNtpTime(xmitTime)
	}

	err = binary.Write(conn, binary.BigEndian, request)
	if err != nil {
		return nil, err
	}

	var response msg
	err = binary.Read(conn, binary.BigEndian, &response)
	if err != nil {
		return nil, err
	}

	if response.OriginTime != request.TransmitTime {
		return nil, errors.New("ntp: origin timestamp mismatch")
	}
	if response.getMode() != server {
		return nil, errors.New("ntp: unexpected response mode")
	}

	recvTime := toNtpTime(xmitTime.Add(time.Since(xmitTime)))
	response.OriginTime = toNtpTime(xmitTime)
	parsed := parseTime(&response, recvTime)
	if err = parsed.Validate(); err != nil {
		return nil, err
	}
	return parsed, nil
}
