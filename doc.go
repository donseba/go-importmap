// Package importmap fetches JavaScript and CSS packages from CDNs and
// generates import maps and stylesheet tags for Go templates.
//
// Vendor downloads the packages at build time, with their licences and a lock
// file, and FromFS loads them from an embedded file system, so the program
// needs no network at run time. CacheOrFetch fetches them at start-up
// instead. Render and FuncMap write the head HTML, and ServeFile serves the
// files with versioned URLs and strong ETags.
package importmap
