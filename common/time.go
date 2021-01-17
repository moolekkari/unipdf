package common

import (
	"time"
)

const timeFormat = "2 January 2006 at 15:04"

func UtcTimeFormat(t time.Time) string {
	return t.Format(timeFormat) + " UTC"
}
