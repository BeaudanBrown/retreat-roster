{
  inputs = {
    nixpkgs.url = "github:cachix/devenv-nixpkgs/rolling";
    systems.url = "github:nix-systems/default";
    devenv.url = "github:cachix/devenv";
    devenv.inputs.nixpkgs.follows = "nixpkgs";
  };

  nixConfig = {
    extra-trusted-public-keys = "devenv.cachix.org-1:w1cLUi8dv3hnoSPGAuibQv+f9TZLr6cv/Hm9XgU50cw=";
    extra-substituters = "https://devenv.cachix.org";
  };

  outputs = { self, nixpkgs, devenv, systems, ... } @ inputs:
  let
    forEachSystem = nixpkgs.lib.genAttrs (import systems);
  in
  {
    packages = forEachSystem (system: {
      devenv-up = self.devShells.${system}.default.config.procfileScript;
    });

    devShells = forEachSystem
    (system:
    let
      pkgs = import nixpkgs {
        inherit system;
        config.allowUnfree = true;
      };
    in
    {
      default = devenv.lib.mkShell {
        inherit inputs pkgs;
        modules = [
          {
            packages = with pkgs; [
              tailwindcss
            ];

            languages = {
              go.enable = true;
              javascript = {
                enable = true;
                npm.enable = true;
              };
            };

            services.mongodb = {
              enable = true;
              initDatabasePassword = "mongodb";
              initDatabaseUsername = "mongodb";
            };

            processes = {
              server.exec = "${pkgs.air}/bin/air";
              tailwind.exec = "tailwindcss -i ./www/input.css -o ./www/app.css --watch=always";
            };

            pre-commit.hooks.gofmt.enable = true;
          }
        ];
      };
    });
  };
}
