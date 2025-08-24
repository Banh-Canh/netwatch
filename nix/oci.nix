{
  pkgs ? import <nixpkgs> { },
  dockerVersion ? "0.0.0",
}:
let
  binaries = pkgs.callPackage ./binaries.nix { };
  frontendAssets = pkgs.stdenv.mkDerivation {
    name = "netwatch-frontend-assets";
    src = pkgs.lib.cleanSource ../.;
    installPhase = ''
      # The final container will have two directories at its root: /static and /templates
      mkdir -p $out/static
      mkdir -p $out/templates

      # Copy the Vite-bundled assets to /static
      cp -r $src/static/* $out/static/

      # Copy the Go HTML templates to /templates
      cp $src/templates/*.html $out/templates/
    '';
  };
  makeDummyImage = {
    fakeRootCommands = ''
      ln -s var/run run
      ln -s bin/${binaries.pname} netwatch
    '';
    name = binaries.pname;
    contents = [
      frontendAssets
      binaries
      pkgs.dockerTools.caCertificates
      pkgs.openssl
      pkgs.cacert
      (pkgs.dockerTools.fakeNss.override {
        extraPasswdLines = [
          "nixbld:x:${toString 1001}:${toString 0}:Build user:/home/${binaries.pname}:/noshell"
        ];
        extraGroupLines = [ "nixbld:!:${toString 1001}:" ];
      })
    ];

    config = {
      User = "1001:0";
      Entrypoint = [ "/netwatch" ];
      Env = [
        "NIX_SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
        "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
      ];
    };
  };
  imageDummy = pkgs.dockerTools.streamLayeredImage {
    inherit (makeDummyImage) fakeRootCommands;
    inherit (makeDummyImage) name;
    inherit (makeDummyImage) contents;
    inherit (makeDummyImage) config;
    tag = "${dockerVersion}";
  };
in
pkgs.dockerTools.streamLayeredImage {
  inherit (makeDummyImage) fakeRootCommands;
  tag = imageDummy.imageTag;
  inherit (makeDummyImage) name;
  inherit (makeDummyImage) contents;
  inherit (makeDummyImage) config;
}
