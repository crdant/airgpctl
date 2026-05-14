// OpenCode Plugin: Enforce Git Hooks
// Prevents skipping git hooks with --no-verify

export const EnforceGitHooks = async ({ project, client, $, directory, worktree }) => {
  const PLUGIN_NAME = "enforce-git-hooks"
  
  return {
    "tool.execute.before": async (input, output) => {
      // Only check bash commands
      if (input.tool !== "bash") return
      
      const command = input.args?.command || ""
      const trimmedCommand = command.trim()
      
      // Check for git commands with --no-verify flag
      const noVerifyPatterns = [
        /git\s+commit\s+.*--no-verify/i,
        /git\s+push\s+.*--no-verify/i,
        /git\s+rebase\s+.*--no-verify/i
      ]
      
      for (const pattern of noVerifyPatterns) {
        if (pattern.test(trimmedCommand)) {
          const errorMessage = `
🚫 Git Hooks Cannot Be Skipped
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
The command attempts to skip git hooks with --no-verify.

This repository relies on pre-commit and pre-push hooks for:
  - Terraform formatting validation
  - Security scanning with tfsec
  - OPA policy enforcement
  - Automatic plan generation

Skipping hooks risks:
  - Unformatted Terraform code being committed
  - Security vulnerabilities going undetected
  - Policy violations being introduced
  - Breaking the CI/CD pipeline

Remove --no-verify and let the hooks run.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`
          
          await client.app.log({
            body: {
              service: PLUGIN_NAME,
              level: "error",
              message: "Blocked command with --no-verify flag"
            }
          })
          
          throw new Error(errorMessage)
        }
      }
    }
  }
}
