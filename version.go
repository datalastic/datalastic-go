package datalastic

// Version is the semantic version of this SDK. The published git tag for a
// release must be exactly "v" + Version (for example v0.2.0).
const Version = "0.2.0"

// userAgent is sent as the User-Agent header on every request.
const userAgent = "datalastic-go/" + Version
