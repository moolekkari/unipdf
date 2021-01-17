// Package common contains common properties used by the subpackages.
package common

import (
	"time"
)

const releaseYear = 2020
const releaseMonth = 4
const releaseDay = 7
const releaseHour = 23
const releaseMin = 40

// Version holds version information, when bumping this make sure to bump the released at stamp also.
const Version = "3.6.0"

var ReleasedAt = time.Date(releaseYear, releaseMonth, releaseDay, releaseHour, releaseMin, 0, 0, time.UTC)
