// OpenCode Plugin: Enforce Branch Naming Convention
// Prevents creating branches that don't follow <type>/<user>/<purpose> format

export const EnforceBranchNaming = async ({ project, client, $, directory, worktree }) => {
  const PLUGIN_NAME = "enforce-branch-naming"
  
  // Valid branch name prefixes
  const validTypes = [
    'build', 'chore', 'ci', 'docs', 'feature', 
    'fix', 'performance', 'refactor', 'revert', 'style', 'test'
  ]
  
  // Pattern: type/user/description (must have at least 3 path segments)
  const branchPattern = new RegExp(`^(${validTypes.join('|')})/[^/]+/.+$`)
  
  return {
    "tool.execute.before": async (input, output) => {
      // Only check git commands
      if (input.tool !== "bash") return
      
      const command = input.args?.command || ""
      const trimmedCommand = command.trim()
      
      // Check for branch creation commands
      const branchCreationPatterns = [
        /^git\s+checkout\s+-b\s+/i,
        /^git\s+switch\s+(-c|--create)\s+/i,
        /^git\s+branch\s+(?!-)([^\s]+)/i
      ]
      
      let branchName = null
      let matchedPattern = false
      
      for (const pattern of branchCreationPatterns) {
        const match = trimmedCommand.match(pattern)
        if (match) {
          matchedPattern = true
          // Extract branch name from the command
          if (pattern.toString().includes('checkout')) {
            branchName = trimmedCommand.replace(/^git\s+checkout\s+-b\s+/, '').split(' ')[0]
          } else if (pattern.toString().includes('switch')) {
            branchName = trimmedCommand.replace(/^git\s+switch\s+(-c|--create)\s+/, '').split(' ')[0]
          } else if (pattern.toString().includes('branch')) {
            branchName = match[1]
          }
          break
        }
      }
      
      if (!matchedPattern || !branchName) return
      
      // Check if branch name follows convention
      if (!branchPattern.test(branchName)) {
        const errorMessage = `
🌿 Branch Naming Convention Violation
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Branch "${branchName}" does not follow the required format.

Required format: <type>/<user>/<purpose>
  - type: ${validTypes.join(', ')}
  - user: your username or identifier
  - purpose: brief description of the change

Examples:
  ✅ feature/crdant/add-vpc-networking
  ✅ fix/crdant/state-bucket-permissions
  ✅ chore/crdant/update-provider-versions
  ❌ ${branchName}

Rename the branch to match the convention.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`
        
        await client.app.log({
          body: {
            service: PLUGIN_NAME,
            level: "warn",
            message: `Blocked branch creation: ${branchName}`
          }
        })
        
        throw new Error(errorMessage)
      }
      
      await client.app.log({
        body: {
          service: PLUGIN_NAME,
          level: "info",
          message: `Branch name validated: ${branchName}`
        }
      })
    }
  }
}
