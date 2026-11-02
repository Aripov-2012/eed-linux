package snapshot

// Policy describes snapshot retention preferences.
type Policy struct {
	DesiredRestorePoints int    `json:"desired_restore_points"`
	EEDRetentionCount    int    `json:"eed_retention_count"`
	RootSubvolume        string `json:"root_subvolume,omitempty"`
	SpaceLimitBytes      int64  `json:"space_limit_bytes"`
	FreeLimitBytes       int64  `json:"free_limit_bytes"`
}
