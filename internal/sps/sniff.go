package sps

import (
	"strings"
)

func sniffSNI(data []byte) string {
	if len(data) < 42 {
		return ""
	}
	if data[0] != 22 {
		return ""
	}
	if int(data[1])<<8|int(data[2]) > len(data) {
		return ""
	}
	off := 5 + 4
	if off+2 > len(data) {
		return ""
	}
	chLen := int(data[off])<<8 | int(data[off+1])
	off += 2
	if off+chLen > len(data) {
		return ""
	}
	off += 32
	if off+1 > len(data) {
		return ""
	}
	sl := int(data[off])
	off++
	if off+sl > len(data) {
		return ""
	}
	off += sl
	if off+2 > len(data) {
		return ""
	}
	off += 2
	for off+4 <= len(data) {
		et := int(data[off])<<8 | int(data[off+1])
		el := int(data[off+2])<<8 | int(data[off+3])
		off += 4
		if off+el > len(data) {
			break
		}
		if et == 0 {
			if el < 2 {
				break
			}
			l := int(data[off])<<8 | int(data[off+1])
			pos := off + 2
			for i := 0; i < l && pos+3 <= len(data); i++ {
				t := data[pos]
				n2 := int(data[pos+1])<<8 | int(data[pos+2])
				pos += 3
				if pos+n2 > len(data) {
					break
				}
				if t == 0 {
					if n2 < 2 {
						break
					}
					vl := int(data[pos])<<8 | int(data[pos+1])
					pos2 := pos + 2
					if pos2+vl > len(data) {
						break
					}
					return string(data[pos2 : pos2+vl])
				}
				pos += n2
			}
			break
		}
		off += el
	}
	return ""
}

func targetForAuthHTTP(host, method, rawURL string) string {
	host = strings.TrimSpace(host)
	if method == "" {
		method = "GET"
	}
	m := strings.ToUpper(method)
	if m == "CONNECT" && host != "" {
		if !strings.Contains(host, "://") {
			return "https://" + host
		}
		return host
	}
	return rawURL
}
