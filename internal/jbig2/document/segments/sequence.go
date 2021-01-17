package segments

// OrganizationType is the enum for the stream sequence organization.
type OrganizationType uint8

// Organization types defined in D.4.2. - File header bit 0 defines the stream sequence organization.
const (
	ORandom OrganizationType = iota
	OSequential
)
