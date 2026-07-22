package authapi

type Result struct {
	UserConns       int64
	IPConns         int64
	UserRate        int64
	IPRate          int64
	UserQPS         int64
	IPQPS           int64
	Upstream        string
	Outgoing        string
	UserTotalRate   int64
	IPTotalRate     int64
	PortTotalRate   int64
	RotationTimeSec int64
	// RejectHTTPStatus: when Authorize returns !ok — 407 auth, 503/429 limit/overload (see docs).
	RejectHTTPStatus int
}
