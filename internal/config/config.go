package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Interfaces     []string `yaml:"interfaces"`
	Ports          []uint16 `yaml:"ports"`
	SampleRatePct  uint32   `yaml:"sample_rate_pct"`
	SubnetPrefixV4 int      `yaml:"subnet_prefix_v4"`
	SubnetPrefixV6 int      `yaml:"subnet_prefix_v6"`
	LogSamplePct   uint32   `yaml:"log_sample_pct"`
	LogLevel       string   `yaml:"log_level"`
	MaxMemory      uint64   `yaml:"max_memory"`
	MaxSamples     int      `yaml:"max_samples"`
	TopN           int      `yaml:"topn"`
	MaxSubnets     int      `yaml:"max_subnets"`
	ListenAddr     string   `yaml:"listen_addr"`
	ConfigFile     string   `yaml:"-"`
}

func Default() Config {
	return Config{
		Interfaces:     []string{},
		Ports:          []uint16{53},
		SampleRatePct:  100,
		SubnetPrefixV4: 24,
		SubnetPrefixV6: 48,
		LogSamplePct:   1,
		LogLevel:       "info",
		MaxMemory:      64 * 1024 * 1024,
		MaxSamples:     100_000,
		TopN:           100,
		MaxSubnets:     10_000,
		ListenAddr:     ":8080",
	}
}

func Load() (Config, error) {
	cfg := Default()

	configFile := ""
	interfaces := ""
	ports := ""
	sampleRate := uint(cfg.SampleRatePct)
	subnetV4 := cfg.SubnetPrefixV4
	subnetV6 := cfg.SubnetPrefixV6
	logSample := uint(cfg.LogSamplePct)
	logLevel := cfg.LogLevel
	maxMemory := cfg.MaxMemory
	maxSamples := cfg.MaxSamples
	topN := cfg.TopN
	maxSubnets := cfg.MaxSubnets
	listenAddr := cfg.ListenAddr

	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.StringVar(&configFile, "config", "", "path to YAML config file")
	fs.StringVar(&interfaces, "interfaces", "", "comma-separated interface names")
	fs.StringVar(&ports, "ports", "", "comma-separated TCP ports")
	fs.UintVar(&sampleRate, "sample-rate-pct", sampleRate, "in-kernel sampling percentage (0-100)")
	fs.IntVar(&subnetV4, "subnet-prefix-v4", subnetV4, "IPv4 prefix length for subnet aggregation")
	fs.IntVar(&subnetV6, "subnet-prefix-v6", subnetV6, "IPv6 prefix length for subnet aggregation")
	fs.UintVar(&logSample, "log-sample-pct", logSample, "percentage of measurements to log (0-100)")
	fs.StringVar(&logLevel, "log-level", logLevel, "log verbosity: debug, info, warn, error")
	fs.Uint64Var(&maxMemory, "max-memory", maxMemory, "maximum memory for in-memory state store (bytes)")
	fs.IntVar(&maxSamples, "max-samples", maxSamples, "maximum raw samples retained")
	fs.IntVar(&topN, "topn", topN, "number of slowest clients to track")
	fs.IntVar(&maxSubnets, "max-subnets", maxSubnets, "maximum distinct subnets tracked")
	fs.StringVar(&listenAddr, "listen-addr", listenAddr, "HTTP listen address")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return cfg, err
	}
	cfg.ConfigFile = configFile

	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return cfg, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config file: %w", err)
		}
	}

	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })

	if visited["interfaces"] {
		cfg.Interfaces = splitCSV(interfaces)
	}
	if visited["ports"] {
		parsed, err := parsePorts(ports)
		if err != nil {
			return cfg, err
		}
		cfg.Ports = parsed
	}
	if visited["sample-rate-pct"] {
		cfg.SampleRatePct = uint32(sampleRate)
	}
	if visited["subnet-prefix-v4"] {
		cfg.SubnetPrefixV4 = subnetV4
	}
	if visited["subnet-prefix-v6"] {
		cfg.SubnetPrefixV6 = subnetV6
	}
	if visited["log-sample-pct"] {
		cfg.LogSamplePct = uint32(logSample)
	}
	if visited["log-level"] {
		cfg.LogLevel = logLevel
	}
	if visited["max-memory"] {
		cfg.MaxMemory = maxMemory
	}
	if visited["max-samples"] {
		cfg.MaxSamples = maxSamples
	}
	if visited["topn"] {
		cfg.TopN = topN
	}
	if visited["max-subnets"] {
		cfg.MaxSubnets = maxSubnets
	}
	if visited["listen-addr"] {
		cfg.ListenAddr = listenAddr
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if len(c.Interfaces) == 0 {
		return errors.New("at least one interface is required")
	}
	if c.SampleRatePct > 100 {
		return errors.New("sample_rate_pct must be 0-100")
	}
	if c.LogSamplePct > 100 {
		return errors.New("log_sample_pct must be 0-100")
	}
	if _, ok := parseLogLevel(c.LogLevel); !ok {
		return fmt.Errorf("log_level must be one of debug, info, warn, error (got %q)", c.LogLevel)
	}
	if c.SubnetPrefixV4 < 0 || c.SubnetPrefixV4 > 32 {
		return errors.New("subnet_prefix_v4 must be 0-32")
	}
	if c.SubnetPrefixV6 < 0 || c.SubnetPrefixV6 > 128 {
		return errors.New("subnet_prefix_v6 must be 0-128")
	}
	if c.MaxSamples <= 0 {
		return errors.New("max_samples must be positive")
	}
	if c.TopN <= 0 {
		return errors.New("topn must be positive")
	}
	if c.MaxSubnets <= 0 {
		return errors.New("max_subnets must be positive")
	}
	if c.ListenAddr == "" {
		return errors.New("listen_addr is required")
	}
	return nil
}

func parseLogLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// SlogLevel returns the configured log level as a slog.Level, defaulting to
// info when the value is unset or unrecognized.
func (c Config) SlogLevel() slog.Level {
	level, _ := parseLogLevel(c.LogLevel)
	return level
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parsePorts(s string) ([]uint16, error) {
	parts := splitCSV(s)
	out := make([]uint16, 0, len(parts))
	for _, p := range parts {
		var port int
		_, err := fmt.Sscanf(p, "%d", &port)
		if err != nil || port < 0 || port > 65535 {
			return nil, fmt.Errorf("invalid port %q", p)
		}
		out = append(out, uint16(port))
	}
	return out, nil
}

func (c Config) PortFilterEnabled() bool {
	return len(c.Ports) > 0
}

func (c Config) SubnetPrefixFor(ip net.IP) int {
	if ip.To4() != nil {
		return c.SubnetPrefixV4
	}
	return c.SubnetPrefixV6
}
