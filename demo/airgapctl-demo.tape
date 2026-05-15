# VHS tape script for airgapctl demo
# Generated GIF will be saved to demo/airgapctl-demo.gif

Set FontSize 18
Set FontFamily "Inconsolata"
Set Width 1200
Set Height 800
Set Theme "rose-pine-moon"
Set TypingSpeed 50ms

# Pre-demo: hide prompt, clear screen, build binary
Hide
Type "clear && make build"
Enter
Sleep 2s
Show

# Show help
Type "./bin/airgapctl --help"
Enter
Sleep 1s

# List images in the bundle
Type "./bin/airgapctl list --bundle demo/fixtures/bundle.airgap"
Enter
Sleep 1s

# Push images to ttl.sh
Type "./bin/airgapctl push --bundle demo/fixtures/bundle.airgap --registry ttl.sh --namespace airgapctl-demo --username unused --password unused"
Enter
Sleep 22s

# Generate values file
Type "./bin/airgapctl values --bundle demo/fixtures/bundle.airgap --chart demo/fixtures/chart --registry myregistry.example.com --namespace airgap --output demo/values-airgap.yaml"
Enter
Sleep 1s

# Show generated values file
Type "cat demo/values-airgap.yaml"
Enter
Sleep 2s

# Wrap up
Type "echo 'Demo complete!'"
Enter
Sleep 500ms

Output demo/airgapctl-demo.gif
