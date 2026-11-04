package service

// Причины, по которым кадр не переведён (frame.data.zorko_error / zorko_site_error; спека, раздел 6)
const (
	CodeNoDebugIdNoRelease = "no_debug_id_no_release"
	CodeMapNotFound        = "map_not_found"
	CodeMapInvalid         = "map_invalid"
	CodePositionNotInMap   = "position_not_in_map"
	CodeStorageUnavailable = "storage_unavailable"
	CodeTimeBudgetExceeded = "time_budget_exceeded"
	CodeSiteDisabled       = "site_disabled"
	CodeSiteHostNotAllowed = "site_host_not_allowed"
	CodeSiteTimeout        = "site_timeout"
	CodeSiteNoSourcemapUrl = "site_no_sourcemap_url"
	CodeSiteTooLarge       = "site_too_large"
	CodeSiteHttpPrefix     = "site_http_"
	CodeSiteUnavailable    = "site_unavailable"

	ViaDebugId = "debug_id"
	ViaRelease = "release"
	ViaSite    = "site"
)
