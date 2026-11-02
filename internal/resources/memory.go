package resources

import "eed/internal/detection"

func AnalyzeRAM(report detection.Report) RAMReport {
	return RAMReport{
		TotalBytes:     report.Hardware.Memory.TotalBytes,
		AvailableBytes: report.Hardware.Memory.AvailableBytes,
		Status:         "unknown",
	}
}
