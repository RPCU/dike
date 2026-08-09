{
  pkgs ? import <nixpkgs> { },
  version ? "dev",
}:
let
  inherit (pkgs.lib) cleanSource cleanSourceWith;
in
pkgs.buildGoModule {
  pname = "dike";
  version = "${version}";

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

  subPackages = [ "cmd" ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  vendorHash = "sha256-cbleaqExozxS7Ba1jPrZknat+KDYzgrPV93myrFvChY=";

  doCheck = false;

  postInstall = ''
    mv $out/bin/cmd $out/bin/dike
    ln -s $out/bin/dike $out/bin/manager
  '';

  meta = with pkgs.lib; {
    description = "$pname; version: $version";
    homepage = "https://github.com/RPCU/dike";
    license = licenses.asl20;
    platforms = platforms.all;
    mainProgram = "dike";
  };
}
