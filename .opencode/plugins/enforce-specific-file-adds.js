// OpenCode Plugin: Enforce Specific File Adds
// Blocks blanket git add commands so files must be staged explicitly

export const EnforceSpecificFileAdds = async ({ project, client, $, directory, worktree }) => {
  const PLUGIN_NAME = "enforce-specific-file-adds"

  // Patterns that indicate "add all" or blanket add operations
  const BLANKET_ADD_PATTERNS = [
    /git\s+add\s+\./i,                    // git add .
    /git\s+add\s+-A/i,                    // git add -A
    /git\s+add\s+--all/i,                // git add --all
    /git\s+add\s+\*/i,                    // git add *
    /git\s+add\s+.*\*\.\*/i,             // git add something with wildcards
    /git\s+commit\s+.*-a/i,               // git commit -a (auto-stages all modified)
    /git\s+commit\s+.*--all/i,           // git commit --all
    /git\s+add\s+-u/i,                   // git add -u (stages all modified/deleted)
    /git\s+add\s+--update/i,             // git add --update
  ]

  return {
    "tool.execute.before": async (input, output) => {
      // Only check bash commands
      if (input.tool !== "bash") return

      const command = input.args?.command || ""
      const trimmedCommand = command.trim()

      // --- BLOCK BLANKET ADD COMMANDS ---
      for (const pattern of BLANKET_ADD_PATTERNS) {
        if (pattern.test(trimmedCommand)) {
          const errorMessage = `
🚫 Blanket File Staging Blocked
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
The command attempts to stage files en masse:

  ${trimmedCommand}

This repository requires an explicit, file-by-file staging
approach. Stage only the files you intend to commit.

❌ Blocked patterns:
   • git add .
   • git add -A / --all
   • git add *
   • git add -u / --update
   • git commit -a / --all

✅ Use explicit file names instead:
   git add <specific-file>
   git commit -m "message"
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`

          await client.app.log({
            body: {
              service: PLUGIN_NAME,
              level: "error",
              message: `Blocked blanket add command: ${trimmedCommand}`,
            },
          })

          throw new Error(errorMessage)
        }
      }
    },
  }
}

      // Glob/wildcard match (e.g., "*.go" matches "main.go")
      if (allowed.includes("*")) {
        const regex = new RegExp(
          "^" + allowed.replace(/\./g, "\\.").replace(/\*/g, ".*") + "$"
        )
        if (regex.test(normalizedPath)) return true
      }
    }
    return false
  }

  return {
    "tool.execute.before": async (input, output) => {
      // Only check bash commands
      if (input.tool !== "bash") return

      const command = input.args?.command || ""
      const trimmedCommand = command.trim()

      // --- BLOCK BLANKET ADD COMMANDS ---
      for (const pattern of BLANKET_ADD_PATTERNS) {
        if (pattern.test(trimmedCommand)) {
          const errorMessage = `
🚫 Blanket File Staging Blocked
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
The command attempts to stage files en masse:

  ${trimmedCommand}

This repository requires commits to be made with an explicit,
file-by-file approach. You may only commit files from the
approved allowlist.

❌ Blocked patterns:
   • git add .
   • git add -A / --all
   • git add *
   • git add -u / --update
   • git commit -a / --all

✅ Use explicit file names instead:
   git add <specific-file>
   git commit -m "message"

Allowed paths/patterns include:
${ALLOWED_FILES.map((f) => "   • " + f).join("\n")}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`

          await client.app.log({
            body: {
              service: PLUGIN_NAME,
              level: "error",
              message: `Blocked blanket add command: ${trimmedCommand}`,
            },
          })

          throw new Error(errorMessage)
        }
      }
    },
  }
}
