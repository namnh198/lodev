{
  description = "lodev: DDEV-like local PHP development with shared Docker services";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      version = "0.1.5"; # the latest release; bump it on every release (`lodev version` shows v<version>)
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
          # When go.mod changes, a nix build fails with `got: sha256-…`: paste that here (README, "Nix").
          vendorHash = "sha256-r8glwA5RlH2rJtggi0vne+nVYrVJ0mFZCEVbGqin1Uk=";
          env.CGO_ENABLED = 0;
          ldflags = [ "-s" "-w" "-X lodev/cmd.Version=v${version}" ];
          nativeBuildInputs = [ pkgs.installShellFiles ];
          # install_test.go runs install.sh, which downloads with curl (file:// URLs, no network).
          nativeCheckInputs = [ pkgs.curl ];
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
