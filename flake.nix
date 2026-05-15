{
  description = "airgapctl - CLI for loading Replicated .airgap bundles into a private registry";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "aarch64-darwin" "x86_64-darwin" "aarch64-linux" "x86_64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.buildGoModule {
            pname = "airgapctl";
            version = "0.1.0";
            src = self;
            vendorHash = null;
          };
        }
      );

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            inputsFrom = [ self.packages.${system}.default ];
            buildInputs = [
              pkgs.gnumake
              pkgs.git
              pkgs.golangci-lint
              pkgs.vhs
              pkgs.ttyd
              pkgs.ffmpeg
            ];
          };
        }
      );
    };
}
