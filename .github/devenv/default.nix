{ pkgs
}:
pkgs.devshell.mkShell (
  { config
  , extraModulesPath
  , ...
  }:
  {
    imports = [
      (extraModulesPath + "/language/go.nix")
    ];

    devshell.packages = [
      pkgs.gopls
    ];

    language.go.package = pkgs.go_1_23;

    env = [
    ];
  }
)
