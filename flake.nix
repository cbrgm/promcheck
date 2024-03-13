{
  description = "promcheck";

  inputs =
    {
      nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";
      gitignore = {
        url = "github:hercules-ci/gitignore.nix";
        inputs.nixpkgs.follows = "nixpkgs";
      };
    };

  outputs = { self, nixpkgs, gitignore }:
    let

      allSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems = f: nixpkgs.lib.genAttrs allSystems (system: f {
        pkgs = import nixpkgs { inherit system; };
      });
    in
    {

      packages = forAllSystems
        ({ pkgs }: {
          default = pkgs.buildGo122Module
            {
              name = "cmd";
              src = gitignore.lib.gitignoreSource ./.;
              vendorHash = null;
            };
        });

      devShells = forAllSystems
        ({ pkgs }: {
          default = pkgs.mkShell {
            shellHook = "exec zsh && echo Welcome to your Go dev shell!";
            packages = with pkgs; [
              gnumake
              go-mockery
              go
              gofumpt
              golangci-lint
              just
              zsh
            ];
          };
        });
    };
}
