package btrfs

import "context"

// Usage is a compatibility alias for the normalized Btrfs usage report.
type Usage = UsageReport

// Analyze runs the read-only usage analysis command and returns the parsed summary.
func Analyze(ctx context.Context, target string) (UsageReport, error) {
	return AnalyzeUsage(ctx, target)
}
