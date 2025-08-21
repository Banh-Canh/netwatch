{
  pkgs ? import <nixpkgs> { },
}:
let
  inherit (pkgs.lib) cleanSource cleanSourceWith;
in
pkgs.buildGoModule {
  pname = "netwatch";
  version = "nix";

  src = cleanSourceWith {
    filter =
      name: _:
      !(
        (baseNameOf name) == "Dockerfile"
        || (baseNameOf name) == "Makefile"
        || (baseNameOf name) == "README.md"
        || (baseNameOf name) == "PROJECT"
        || (baseNameOf name) == "config"
        || (baseNameOf name) == "conf"
        || (baseNameOf name) == "nix"
      );
    src = cleanSource ../.;
  };

  vendorHash = "sha256-SbC3ya8K7RKBG2t6qqVJjHviCuYpXxijz5Q4oFGE+Gg=";

  doCheck = true;

  meta = with pkgs.lib; {
    description = "$pname; version: $version";
    homepage = "http://github.com/banh-canh/$pname";
    license = licenses.asl20;
    platforms = platforms.linux;
    mainProgram = "$pname";
  };
}
