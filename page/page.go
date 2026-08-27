/*
 * Copyright © 2018-2019, 2021 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

// package page provides mechanisms and data for generating a WriteFreely page.
package page

import (
	"strings"

	"github.com/writefreely/writefreely/config"
)

type StaticPage struct {
	// App configuration
	config.AppCfg
	Version string
	// SoftwareName is the product name as shown to people, including any
	// fork edition.
	SoftwareName string
	// SoftwareURL is where this build's source can be obtained.
	SoftwareURL string
	// UpstreamVersion is the WriteFreely release this build is based on,
	// used for links into WriteFreely's versioned documentation.
	UpstreamVersion string
	HeaderNav       bool
	CustomCSS       bool

	// Request values
	Path            string
	Username        string
	Values          map[string]string
	Flashes         []string
	CanViewReader   bool
	IsAdmin         bool
	CanInvite       bool
	UpdateAvailable bool
}

// SanitizeHost alters the StaticPage to contain a real hostname. This is
// especially important for the Tor hidden service, as it can be served over
// proxies, messing up the apparent hostname.
func (sp *StaticPage) SanitizeHost(cfg *config.Config) {
	if cfg.Server.HiddenHost != "" && strings.HasPrefix(sp.Host, cfg.Server.HiddenHost) {
		sp.Host = cfg.Server.HiddenHost
	}
}

// OfficialVersion returns the version to use when linking into
// WriteFreely's versioned documentation. A fork's own version number does
// not exist upstream, so those links must point at the release this build
// is based on.
func (sp StaticPage) OfficialVersion() string {
	if sp.UpstreamVersion != "" {
		return sp.UpstreamVersion
	}
	p := strings.Split(sp.Version, "-")
	return p[0]
}
