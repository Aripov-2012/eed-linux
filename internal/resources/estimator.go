package resources

import "eed/internal/detection"

func EstimateStorage(report detection.Report) (int64, bool) {
	var total int64
	for _, item := range report.System.Storage {
		total += item.AvailableBytes
	}
	return total, true
}
