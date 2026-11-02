package portalconfig

type Plan struct {
	Desktop                string   `json:"desktop"`
	ConfigPath             string   `json:"config_path"`
	Content                string   `json:"content"`
	RequiredPackages       []string `json:"required_packages"`
	PackageInventoryStatus string   `json:"package_inventory_status"`
	MissingPackages        []string `json:"missing_packages,omitempty"`
	ExistingConfigPaths    []string `json:"existing_config_paths,omitempty"`
	Status                 string   `json:"status"`
	PreserveExisting       bool     `json:"preserve_existing"`
	UserConfigDetected     bool     `json:"user_config_detected"`
	Environment            []string `json:"environment"`
	Limitations            []string `json:"limitations,omitempty"`
}
