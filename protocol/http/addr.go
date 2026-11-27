package http

import (
	"net/http"
	"strings"

	M "github.com/metacubex/sing/common/metadata"
)

// SourceAddress parses the transport RemoteAddr without trusting unverified X-Forwarded-For headers.
func SourceAddress(request *http.Request) M.Socksaddr {
	return M.ParseSocksaddr(request.RemoteAddr).Unwrap()
}

// SourceAddressFromForwarded parses X-Forwarded-For if the caller explicitly trusts an upstream proxy.
func SourceAddressFromForwarded(request *http.Request) M.Socksaddr {
	address := M.ParseSocksaddr(request.RemoteAddr)
	forwardFrom := request.Header.Get("X-Forwarded-For")
	if forwardFrom != "" {
		for _, from := range strings.Split(forwardFrom, ",") {
			originAddr := M.ParseAddr(strings.TrimSpace(from))
			if originAddr.IsValid() {
				address.Addr = originAddr
				break
			}
		}
	}
	return address.Unwrap()
}
