package gcp

import "github.com/taknb2nch/harelog"

// WithCloudLogging configures the logger for Google Cloud Logging by setting the appropriate entry modifier and JSON key overrides.
func WithCloudLogging(projectID string) harelog.Option {
	return func(l *harelog.Logger) {
		harelog.WithEntryModifier(func(e *harelog.LogEntry) {
			if projectID != "" {
				if e.TraceID != "" {
					e.TraceID = "projects/" + projectID + "/traces/" + e.TraceID
				}

				e.Payload["projectId"] = projectID
			} else {
				delete(e.Payload, "projectId")
			}
		})(l)

		harelog.WithKeyOverride(harelog.FieldKeyTraceID, "logging.googleapis.com/trace")(l)
		harelog.WithKeyOverride(harelog.FieldKeySpanID, "logging.googleapis.com/spanId")(l)
		harelog.WithKeyOverride(harelog.FieldKeyTraceSampled, "logging.googleapis.com/trace_sampled")(l)
		harelog.WithKeyOverride(harelog.FieldKeySourceLocation, "logging.googleapis.com/sourceLocation")(l)
	}
}
