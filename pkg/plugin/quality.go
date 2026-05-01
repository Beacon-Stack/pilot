// Package plugin defines the public interfaces and value types for
// Pilot's plugin system. Indexer, DownloadClient, and Notifier
// implementations (built-in or external) depend only on this package.
package plugin

// Protocol identifies the release download mechanism.
type Protocol string

const (
	ProtocolTorrent Protocol = "torrent"
	ProtocolNZB     Protocol = "nzb"
	ProtocolUnknown Protocol = "unknown"
)

// Resolution is the video resolution of a release.
type Resolution string

const (
	ResolutionUnknown Resolution = "unknown"
	ResolutionSD      Resolution = "sd" // implied SD (e.g. DVDRip without explicit resolution)
	Resolution480p    Resolution = "480p"
	Resolution576p    Resolution = "576p"
	Resolution720p    Resolution = "720p"
	Resolution1080p   Resolution = "1080p"
	Resolution2160p   Resolution = "2160p" // 4K
)

// Source is the origin/format of a release.
type Source string

const (
	SourceUnknown   Source = "unknown"
	SourceWorkprint Source = "workprint"
	SourceCAM       Source = "cam"
	SourceTelesync  Source = "telesync"
	SourceTELECINE  Source = "telecine"
	SourceDVDSCR    Source = "dvdscr"
	SourceRegional  Source = "regional"
	SourceDVD       Source = "dvd"
	SourceDVDR      Source = "dvdr"
	SourceHDTV      Source = "hdtv"
	SourceWEBRip    Source = "webrip"
	SourceWEBDL     Source = "webdl"
	SourceBluRay    Source = "bluray"
	SourceRemux     Source = "remux"
	SourceBRDisk    Source = "brdisk"
	SourceRawHD     Source = "rawhd"
)

// Codec is the video codec of a release.
type Codec string

const (
	CodecUnknown Codec = "unknown"
	CodecX264    Codec = "x264"
	CodecX265    Codec = "x265"
	CodecAV1     Codec = "av1"
	CodecXVID    Codec = "xvid"
)

// HDRFormat is the high dynamic range format of a release.
type HDRFormat string

const (
	HDRNone        HDRFormat = "none"
	HDRUnknown     HDRFormat = "unknown"
	HDRHDR10       HDRFormat = "hdr10"
	HDRDolbyVision HDRFormat = "dolby_vision"
	HDRHLG         HDRFormat = "hlg"
	HDRHDR10Plus   HDRFormat = "hdr10plus"
)

// AudioCodec is the audio codec of a release.
type AudioCodec string

const (
	AudioCodecUnknown     AudioCodec = ""
	AudioCodecTrueHD      AudioCodec = "truehd"
	AudioCodecTrueHDAtmos AudioCodec = "truehd_atmos"
	AudioCodecDTSX        AudioCodec = "dts_x"
	AudioCodecDTSHDMA     AudioCodec = "dts_hd_ma"
	AudioCodecDTSHD       AudioCodec = "dts_hd"
	AudioCodecDTS         AudioCodec = "dts"
	AudioCodecEAC3Atmos   AudioCodec = "eac3_atmos"
	AudioCodecEAC3        AudioCodec = "eac3"
	AudioCodecAC3         AudioCodec = "ac3"
	AudioCodecAAC         AudioCodec = "aac"
	AudioCodecFLAC        AudioCodec = "flac"
	AudioCodecPCM         AudioCodec = "pcm"
	AudioCodecMP3         AudioCodec = "mp3"
	AudioCodecOpus        AudioCodec = "opus"
)

// AudioChannels is the channel layout of a release.
type AudioChannels string

const (
	AudioChannelsUnknown AudioChannels = ""
	AudioChannels71      AudioChannels = "7.1"
	AudioChannels51      AudioChannels = "5.1"
	AudioChannels20      AudioChannels = "2.0"
	AudioChannels10      AudioChannels = "1.0"
)

// Quality describes the technical characteristics of a release.
// It is a value type — embedded in releases, files, and profiles.
type Quality struct {
	Resolution    Resolution    `json:"resolution"`
	Source        Source        `json:"source"`
	Codec         Codec         `json:"codec"`
	HDR           HDRFormat     `json:"hdr"`
	AudioCodec    AudioCodec    `json:"audio_codec,omitempty"`
	AudioChannels AudioChannels `json:"audio_channels,omitempty"`
	// Name is the human-readable label derived from the video fields,
	// e.g. "Bluray-1080p x265" or "WEBDL-2160p HDR10".
	Name string `json:"name"`
}

// Score returns a numeric rank for this quality used when comparing
// two qualities. Higher is better.
func (q Quality) Score() int {
	return resolutionScore(q.Resolution)*100 + sourceScore(q.Source)*10 + codecScore(q.Codec)
}

// BetterThan reports whether q is strictly better than other.
func (q Quality) BetterThan(other Quality) bool {
	return q.Score() > other.Score()
}

// AtLeast reports whether q meets or exceeds other.
func (q Quality) AtLeast(other Quality) bool {
	return q.Score() >= other.Score()
}

func resolutionScore(r Resolution) int {
	switch r {
	case Resolution2160p:
		return 4
	case Resolution1080p:
		return 3
	case Resolution720p:
		return 2
	case Resolution576p, Resolution480p, ResolutionSD:
		return 1
	default:
		return 0
	}
}

func sourceScore(s Source) int {
	switch s {
	case SourceRawHD:
		return 9
	case SourceBRDisk:
		return 8
	case SourceRemux:
		return 7
	case SourceBluRay:
		return 6
	case SourceWEBDL:
		return 5
	case SourceWEBRip:
		return 4
	case SourceHDTV:
		return 3
	case SourceDVD, SourceDVDR:
		return 2
	case SourceDVDSCR, SourceRegional, SourceTELECINE:
		return 1
	case SourceTelesync, SourceCAM:
		return 0
	case SourceWorkprint:
		return 0
	default:
		return 0
	}
}

func codecScore(c Codec) int {
	switch c {
	case CodecAV1:
		return 3
	case CodecX265:
		return 2
	case CodecX264:
		return 1
	default:
		return 0
	}
}

// IndexerFlag represents a flag reported by an indexer for a release.
type IndexerFlag string

const (
	FlagFreeleech    IndexerFlag = "freeleech"
	FlagHalfleech    IndexerFlag = "halfleech"
	FlagFreeleech75  IndexerFlag = "freeleech_75"
	FlagFreeleech25  IndexerFlag = "freeleech_25"
	FlagDoubleUpload IndexerFlag = "double_upload"
	FlagInternal     IndexerFlag = "internal"
	FlagScene        IndexerFlag = "scene"
	FlagNuked        IndexerFlag = "nuked"
)

// ScoreBreakdown records how a release was evaluated against a quality profile.
// Each dimension is independently scored; Total is the sum.
type ScoreBreakdown struct {
	Total      int              `json:"total"`
	Dimensions []ScoreDimension `json:"dimensions"`
	// CustomFormatScore is the sum of matched custom format scores for the
	// quality profile. It sits alongside (not inside) Total because CF scoring
	// is an independent dimension used for separate thresholds.
	CustomFormatScore int      `json:"custom_format_score"`
	MatchedFormats    []string `json:"matched_formats,omitempty"`
	// EditionBonus is the bonus points awarded when the release edition
	// matches the series' preferred edition (+30 pts). It is additive
	// to Total and reflected in the Dimensions list.
	EditionBonus int `json:"edition_bonus"`
}

// ScoreDimension is one component of a ScoreBreakdown.
type ScoreDimension struct {
	Name    string `json:"name"`    // "resolution", "source", "codec", "hdr"
	Score   int    `json:"score"`   // points awarded for this dimension
	Max     int    `json:"max"`     // maximum possible for this dimension
	Matched bool   `json:"matched"` // did it meet the profile requirement?
	Got     string `json:"got"`     // what we found (e.g. "x264")
	Want    string `json:"want"`    // what the profile requires (e.g. "x265")
}

// Release is the transient result of an indexer search.
// It is not stored in the database. A summary is written to
// GrabHistory when a release is grabbed.
type Release struct {
	GUID         string
	Title        string
	Indexer      string
	Protocol     Protocol
	DownloadURL  string
	InfoURL      string
	Size         int64
	Seeds        int
	Peers        int
	AgeDays      float64
	Quality      Quality
	Edition      string // canonical edition name parsed from title; empty = untagged
	ReleaseGroup string // scene group name parsed from title; empty = unknown
	IndexerFlags []IndexerFlag

	// Media context — populated by Pilot/Prism before sending to download client.
	// Download clients that support renaming (Haul) use these fields.
	MediaType     string `json:"media_type,omitempty"`     // "movie" or "tv"
	MediaTitle    string `json:"media_title,omitempty"`    // series or movie title
	MediaYear     int    `json:"media_year,omitempty"`     // release year
	SeasonNumber  int    `json:"season_number,omitempty"`  // TV only
	EpisodeNumber int    `json:"episode_number,omitempty"` // TV only
	EpisodeTitle  string `json:"episode_title,omitempty"`  // TV only

	// Arr-side IDs forwarded to history-aware download clients (Haul)
	// so they can answer "have I downloaded anything for series X?"
	// later. All optional; leaving empty disables the lookup integration
	// for this grab.
	TMDBID    int    `json:"tmdb_id,omitempty"`    // TMDB tv/movie id
	SeriesID  string `json:"series_id,omitempty"`  // Pilot series UUID
	EpisodeID string `json:"episode_id,omitempty"` // Pilot episode UUID
}
