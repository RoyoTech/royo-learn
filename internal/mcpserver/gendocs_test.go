package mcpserver

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Generated MCP reference (plan §Tramo 6: "Generar o validar automáticamente
// ... No copiar la misma lista a mano en cinco documentos").
//
// docs/generated/MCP_REFERENCE.md and docs/generated/PROFILES.md are DERIVED
// from `allTools`, the same registry the server binds. Nobody edits them by
// hand, so they cannot drift from the tools the binary actually serves — which
// is how v0.1.9 shipped documentation describing tools that did not exist.
//
// Regenerate with:
//
//	go test ./cmd/royo-learn ./internal/domain ./internal/mcpserver -update-docs
// ---------------------------------------------------------------------------

func TestGeneratedMCPReferenceIsCurrent(t *testing.T) {
	assertGeneratedDoc(t, "MCP_REFERENCE.md", buildMCPReference())
}

func TestGeneratedProfilesIsCurrent(t *testing.T) {
	assertGeneratedDoc(t, "PROFILES.md", buildProfilesReference())
}

func buildMCPReference() string {
	var sb strings.Builder
	sb.WriteString(generatedHeader("internal/mcpserver/gendocs_test.go", "el registro `allTools`"))
	sb.WriteString("# Referencia MCP\n\n")
	sb.WriteString(fmt.Sprintf("Herramientas canónicas registradas: **%d**.\n\n", len(allTools)))
	sb.WriteString("| Herramienta | Acceso | Perfiles | Aliases deprecated | Descripción |\n")
	sb.WriteString("|-------------|--------|----------|--------------------|-------------|\n")

	for _, tool := range allTools {
		profiles := make([]string, 0, len(tool.profiles))
		for p, on := range tool.profiles {
			if on {
				profiles = append(profiles, p)
			}
		}
		sort.Strings(profiles)

		aliases := "—"
		if len(tool.aliases) > 0 {
			quoted := make([]string, 0, len(tool.aliases))
			for _, a := range tool.aliases {
				quoted = append(quoted, "`"+a+"`")
			}
			aliases = strings.Join(quoted, ", ")
		}

		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
			tool.name,
			accessLabel(tool.access),
			strings.Join(profiles, ", "),
			aliases,
			strings.ReplaceAll(tool.description, "|", "\\|"),
		))
	}

	sb.WriteString("\n## Aliases deprecated\n\n")
	sb.WriteString("Los aliases siguen siendo invocables y comparten el handler de su nombre\n")
	sb.WriteString("canónico, pero **no se anuncian** en las instrucciones del servidor (D14, D16).\n")
	return sb.String()
}

func buildProfilesReference() string {
	var sb strings.Builder
	sb.WriteString(generatedHeader("internal/mcpserver/gendocs_test.go", "el registro `allTools`"))
	sb.WriteString("# Perfiles MCP\n\n")
	sb.WriteString("Cada perfil sirve un subconjunto del registro. Nada destructivo aparece en\n")
	sb.WriteString("`read` ni en `agent` (D2), y esa regla la impone una prueba de contrato\n")
	sb.WriteString("permanente, no esta tabla.\n\n")

	for _, profile := range []string{profileRead, profileAgent, profileAdmin} {
		tools := profileTools(profile)
		sb.WriteString(fmt.Sprintf("## `%s`\n\n", profile))
		sb.WriteString(fmt.Sprintf("Herramientas: **%d**.\n\n", len(tools)))
		for _, tool := range tools {
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", tool.name, accessLabel(tool.access)))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Nombres de perfil deprecated\n\n")
	sb.WriteString("| Deprecated | Canónico |\n|------------|----------|\n")
	deprecated := make([]string, 0, len(deprecatedProfiles))
	for name := range deprecatedProfiles {
		deprecated = append(deprecated, name)
	}
	sort.Strings(deprecated)
	for _, name := range deprecated {
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", name, deprecatedProfiles[name]))
	}
	return sb.String()
}

func accessLabel(a toolAccess) string {
	switch a {
	case accessRead:
		return "read"
	case accessWrite:
		return "write"
	case accessDestructive:
		return "**destructive**"
	default:
		return "unknown"
	}
}
