package videoserver

import (
	"net/http"
	"time"

	"github.com/LdDl/video-server/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// TimeRange represents a continuous time range of available archive
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ArchiveRangesResponse is the API response for available time ranges
type ArchiveRangesResponse struct {
	StreamID string      `json:"stream_id"`
	Ranges   []TimeRange `json:"ranges"`
}

// ArchiveRangesWrapper returns handler for listing available archive time ranges
func ArchiveRangesWrapper(app *Application, verboseLevel VerboseLevel) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		streamIDStr := ctx.Param("stream_id")
		streamID, err := uuid.Parse(streamIDStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid stream_id"})
			return
		}

		// Get archive storage (stream-specific or global fallback)
		archive, err := app.GetArchiveStorageForPlayback(streamID)
		if err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "archive not available: " + err.Error()})
			return
		}

		// List all segments (use a wide time range)
		startTime := time.Unix(0, 0)
		endTime := time.Now().Add(24 * time.Hour)

		segments, err := archive.store.ListFiles(ctx.Request.Context(), archive.filesystemDir, streamID.String(), startTime, endTime)
		if err != nil {
			if verboseLevel > VERBOSE_NONE {
				log.Error().Err(err).Str("stream_id", streamIDStr).Msg("Failed to list archive files")
			}
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list archive files"})
			return
		}

		if len(segments) == 0 {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "no archive segments found"})
			return
		}

		// Calculate time ranges from segments
		segmentDuration := time.Duration(archive.msPerSegment) * time.Millisecond
		ranges := calculateTimeRanges(segments, segmentDuration)

		ctx.JSON(http.StatusOK, ArchiveRangesResponse{
			StreamID: streamIDStr,
			Ranges:   ranges,
		})
	}
}

// calculateTimeRanges groups segments into continuous time ranges
func calculateTimeRanges(segments []storage.ArchiveSegmentInfo, segmentDuration time.Duration) []TimeRange {
	if len(segments) == 0 {
		return nil
	}

	var ranges []TimeRange
	var currentRange *TimeRange

	// Gap threshold: if segments are more than 2x segment duration apart, start new range
	gapThreshold := segmentDuration * 2

	for _, seg := range segments {
		if currentRange == nil {
			currentRange = &TimeRange{Start: seg.StartTime, End: seg.StartTime.Add(segmentDuration)}
			continue
		}

		// Check if this segment is close enough to extend current range
		if seg.StartTime.Sub(currentRange.End) <= gapThreshold {
			currentRange.End = seg.StartTime.Add(segmentDuration)
		} else {
			// Gap detected, save current range and start new one
			ranges = append(ranges, *currentRange)
			currentRange = &TimeRange{Start: seg.StartTime, End: seg.StartTime.Add(segmentDuration)}
		}
	}

	if currentRange != nil {
		ranges = append(ranges, *currentRange)
	}

	return ranges
}
