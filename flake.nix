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
            pkgs = nixpkgs.legacyPackages.${system};
          in
          {
            default = devenv.lib.mkShell {
              inherit inputs pkgs;
              modules = [
                {
                  packages = [ pkgs.hello ];

                  languages.go.enable = true;

                  services.postgres = {
                    enable = true;
                    initialDatabases = [{
                      name = "rosterdb";
                    }];
                    listen_addresses = "127.0.0.1";
                    port = 16969;
                    initialScript = ''
CREATE TABLE staff (
    id UUID PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE rosters (
    id UUID PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE TABLE timesheets (
    id UUID PRIMARY KEY,
    data JSONB NOT NULL
);

CREATE INDEX idx_staff_data ON staff USING gin (data);
CREATE INDEX idx_rosters_data ON rosters USING gin (data);
CREATE INDEX idx_timesheets_data ON timesheets USING gin (data);
CREATE USER rosterdb;
GRANT ALL PRIVILEGES ON DATABASE rosterdb TO rosterdb;
                    '';
                  };

                }
              ];
            };
          });
    };
}
