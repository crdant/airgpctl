// OpenCode Plugin: Enforce Signed Commits
// Requires GPG signing on all commits

export const EnforceSignedCommits = async ({ project, client, $, directory, worktree }) => {
  const PLUGIN_NAME = "enforce-signed-commits"
  
  return {
    "tool.execute.before": async (input, output) => {
      // Only check bash commands
      if (input.tool !== "bash") return
      
      const command = input.args?.command || ""
      const trimmedCommand = command.trim()
      
      // Check for git commit commands that disable signing
      const unsignedPatterns = [
        /git\s+commit\s+.*--no-gpg-sign/i,
        /git\s+commit\s+.*-c\s+commit\.gpgsign=false/i,
        /git\s+commit\s+.*--gpg-sign=false/i
      ]
      
      for (const pattern of unsignedPatterns) {
        if (pattern.test(trimmedCommand)) {
          const errorMessage = `
🔒 All Commits Must Be Signed
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
The command attempts to disable GPG signing.

This repository requires all commits to be signed for:
  - Attribution verification
  - Code integrity guarantees
  - Compliance with security policies

Remove:
  ❌ --no-gpg-sign
  ❌ -c commit.gpgsign=false
  ❌ --gpg-sign=false

Configure GPG signing if not already set up:
  git config --global user.signingkey <KEY_ID>
  git config --global commit.gpgsign true

Or use the signed commit helper:
  git commit -S -m "Your message"
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`
          
          await client.app.log({
            body: {
              service: PLUGIN_NAME,
              level: "error",
              message: "Blocked unsigned commit attempt"
            }
          })
          
          throw new Error(errorMessage)
        }
      }
      
      // Check if this is a commit command without signing
      // Only warn for new commits (not amend which might already be signed)
      if (/^git\s+commit\s+/i.test(trimmedCommand) && 
          !/(-S|--gpg-sign(=true)?)\s/i.test(trimmedCommand) &&
          !/--amend/i.test(trimmedCommand)) {
        
        // Check if repo requires signing (check git config)
        try {
          const gpgSign = await $`git config --get commit.gpgsign`.text().catch(() => "false")
          if (gpgSign.trim() !== "true") {
            await client.app.log({
              body: {
                service: PLUGIN_NAME,
                level: "warn",
                message: "Commit without GPG signing detected. This repository requires signed commits."
              }
            })
            
            // Don't block, just warn - the user may have signing configured differently
            // The pre-commit hook will catch this if properly installed
          }
        } catch {
          // Ignore errors from git config check
        }
      }
    }
  }
}
