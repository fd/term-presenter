{
  description = "Write and record terminal session scripts";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-24.05";
    devshell.url = "github:numtide/devshell";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils, devshell }:
    flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-linux" ] (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [
            devshell.overlays.default
          ];
        };
      in
      {
        packages.default = pkgs.buildGo123Module {
          name = "term-presenter";
          vendorHash = null;
          src = pkgs.lib.fileset.toSource {
            root = ./.;
            fileset = with pkgs.lib.fileset; unions [
              (fileFilter (file: file.hasExt "go") ./.)

              # Go dependencies
              ./go.mod
              ./go.sum
              ./vendor
            ];
          };
        };

        devShells.default = import ./.github/devenv/default.nix { inherit pkgs; };
      });
}
