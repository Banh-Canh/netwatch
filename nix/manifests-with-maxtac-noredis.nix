{
  pkgs ? import <nixpkgs> { },
  imageName ? "netwatch",
  imageTag ? "v0.0.0",
  src ? ../manifests,
}:
let
  maxTacVersion = "v0.3.0";
  maxtacBundle = pkgs.fetchurl {
    url = "https://github.com/Banh-Canh/Maxtac/releases/download/${maxTacVersion}/bundle.yaml";
    sha256 = "sha256-QgLVhZTohKfGcNFpVGYbt2TAWa+fs9H4yi0KFmeDhJQ=";
  };
  runCommand =
    pkgs.runCommand "manifests"
      {
        name = "kustomize";
        nativeBuildInputs = [ pkgs.kustomize ];
        src = pkgs.lib.cleanSource src;
      }
      ''
        set -e # Exit immediately if a command exits with a non-zero status.

        echo "--> Copying source to ./config"
        mkdir ./netwatch -p
        cp -r ${src}/* ./netwatch/
        cp ${maxtacBundle} ./maxtac.yaml

        echo "--> Setting image to ${imageName}:${imageTag}"
        kustomize init
        kustomize edit add resource ./netwatch
        kustomize edit add resource maxtac.yaml
        kustomize edit set image netwatch=${imageName}:${imageTag}
        kustomize edit set image netwatch-cleanup-controller=${imageName}:${imageTag}

        echo "--> Building kustomize output into $out/bundle.yaml"
        mkdir -p $out # Ensure the output directory exists
        kustomize build ./ > $out/bundle.yaml

        echo "--> Build complete."
      '';
in
runCommand
