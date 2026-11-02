package packageinfo

import "context"

const (
	StateAlreadyInstalled      = "already_installed"
	StateNotInstalled          = "not_installed"
	StateVersionMismatch       = "version_mismatch"
	StateRepositoryUnavailable = "repository_unavailable"
	StateUnknown               = "unknown"
)

type Package struct {
	Name               string   `json:"name"`
	Version            string   `json:"version,omitempty"`
	Repository         string   `json:"repository,omitempty"`
	Architecture       string   `json:"architecture,omitempty"`
	State              string   `json:"state"`
	Installed          bool     `json:"installed"`
	Explicit           bool     `json:"explicit"`
	ExplicitKnown      bool     `json:"explicit_known"`
	InstalledVersion   string   `json:"installed_version,omitempty"`
	DownloadBytes      int64    `json:"download_bytes,omitempty"`
	DownloadSizeKnown  bool     `json:"download_size_known"`
	InstalledBytes     int64    `json:"installed_bytes,omitempty"`
	InstalledSizeKnown bool     `json:"installed_size_known"`
	Dependencies       []string `json:"dependencies,omitempty"`
	OptionalDeps       []string `json:"optional_dependencies,omitempty"`
	BuildDependencies  []string `json:"build_dependencies,omitempty"`
	CheckDependencies  []string `json:"check_dependencies,omitempty"`
	Provides           []string `json:"provides,omitempty"`
	Conflicts          []string `json:"conflicts,omitempty"`
	Replaces           []string `json:"replaces,omitempty"`
}

type Candidate struct {
	Name              string `json:"name"`
	State             string `json:"state"`
	RepositoryStatus  string `json:"repository_status"`
	InstalledVersion  string `json:"installed_version,omitempty"`
	RepositoryVersion string `json:"repository_version,omitempty"`
	Reason            string `json:"reason,omitempty"`
	Explicit          *bool  `json:"explicit,omitempty"`
}

type InstalledPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Report struct {
	Status                   string             `json:"status"`
	MetadataStatus           string             `json:"metadata_status"`
	ProviderResolutionStatus string             `json:"provider_resolution_status"`
	InventoryKnown           bool               `json:"inventory_known"`
	TotalDownloadKnown       bool               `json:"total_download_known"`
	TotalInstalledKnown      bool               `json:"total_installed_known"`
	AdditionalDownloadKnown  bool               `json:"additional_download_known"`
	AdditionalInstallKnown   bool               `json:"additional_install_known"`
	RepositoryDatabaseAge    string             `json:"repository_database_age,omitempty"`
	RepositoryDatabaseStale  bool               `json:"repository_database_stale"`
	Candidates               []Candidate        `json:"candidates"`
	InstalledPackages        []InstalledPackage `json:"installed_packages,omitempty"`
	ResolvedPackages         []Package          `json:"resolved_packages,omitempty"`
	UnknownDependencies      []string           `json:"unknown_dependencies,omitempty"`
	Warnings                 []string           `json:"warnings,omitempty"`
	Errors                   []string           `json:"errors,omitempty"`
	TotalDownloadBytes       int64              `json:"total_download_bytes"`
	TotalInstalledBytes      int64              `json:"total_installed_bytes"`
	AdditionalDownloadBytes  int64              `json:"additional_download_bytes"`
	AdditionalInstallBytes   int64              `json:"additional_install_bytes"`
	EstimateComplete         bool               `json:"estimate_complete"`
}

type Inspector interface {
	Inspect(context.Context, []string) (Report, error)
}

func UnavailableReport(packages []string, status, reason string) Report {
	report := Report{
		Status: status, MetadataStatus: "unavailable", ProviderResolutionStatus: "unknown", EstimateComplete: false,
		Candidates: []Candidate{}, Warnings: []string{},
	}
	if reason != "" {
		report.Warnings = append(report.Warnings, reason)
	}
	for _, name := range packages {
		report.Candidates = append(report.Candidates, Candidate{
			Name: name, State: StateUnknown, RepositoryStatus: "unknown", Reason: reason,
		})
	}
	return report
}
