package resources

import "eed/internal/detection"

func AnalyzeStorage(report detection.Report) StorageReport {
	return StorageReport{
		AvailableBytes: sumAvailableStorage(report.System.Storage),
		Status:         "unknown",
	}
}
