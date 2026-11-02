package resources

import "eed/internal/detection"

func AnalyzeCPU(report detection.Report) CPUReport {
	return CPUReport{
		LogicalCores:  report.Hardware.CPU.LogicalCores,
		PhysicalCores: report.Hardware.CPU.PhysicalCores,
		Status:        "unknown",
	}
}
