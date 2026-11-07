{
  description = "lodev: DDEV-like local PHP development with shared Docker services";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      version = "0.0.0-dev"; # bump on release; `lodev version` shows it
      systems = [ "aarch64-darwin" "x86_64-darwin" "aarch64-linux" "x86_64-linux" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAll (pkgs: {
        # go.mod needs a recent Go; buildGoLatestModule tracks nixpkgs' newest toolchain.
        default = pkgs.buildGoLatestModule {
          pname = "lodev";
          inherit version;
          src = ./.;
          subPackages = [ "." ];
          # First build fails with the real hash; paste it here (see README, "Nix").
          vendorHash = "sha256-r8glwA5RlH2rJtggi0vne+nVYrVJ0mFZCEVbGqin1Uk="; # update when go.mod changes: the `got:` hash from a failing nix build
          env.CGO_ENABLED = 0;
          ldflags = [ "-s" "-w" "-X lodev/cmd.Version=${version}" ];
          nativeBuildInputs = [ pkgs.installShellFiles ];
          postInstall = ''
            installShellCompletion --cmd lodev \
              --bash <($out/bin/lodev completion bash) \
              --zsh <($out/bin/lodev completion zsh) \
              --fish <($out/bin/lodev completion fish)
          '';
          meta = {
            description = "DDEV-like local PHP development with shared Docker services";
            homepage = "https://github.com/namnh198/lodev";
            license = pkgs.lib.licenses.mit;
            mainProgram = "lodev";
            platforms = systems;
          };
        };
      });

      apps = forAll (pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${pkgs.system}.default}/bin/lodev";
        };
      });
    };
}
