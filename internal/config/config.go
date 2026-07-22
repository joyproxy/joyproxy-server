package config

import "time"

type SPS struct {
	ServerType    string
	Transport     string
	PortSpec      string
	GatewayIP     string
	Forever       bool
	Daemon        bool
	Worker        bool
	Restart       time.Duration
	AuthNoUser    bool
	AuthURL       string
	AuthCache     int
	AuthFailCache int
	MaxConnsRate  int
	TrafficURL    string
	TrafficMode   string
	SniffDomain   bool
	DefaultParent string
	// LogLevel: logx.LevelSilent | LevelErrorsOnly | LevelQuiet | LevelNormal | LevelVerbose
	LogLevel int
}
