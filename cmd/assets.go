package cmd

import _ "embed"

// manifestTemplate is compiled into the binary so runtime does not depend on
// template files in the target monorepo.
//go:embed manifest.template.js
var manifestTemplate string
