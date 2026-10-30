"""Generates the shelf test fixtures. Usage: python3 testdata/gen_fixture.py testdata"""
import hashlib, json, os, sys

root = sys.argv[1]
REPO = "conan-local"

def h(s, n=40):
    return hashlib.sha1(s.encode()).hexdigest()[:n]

LINUX = {"os": "Linux", "arch": "x86_64", "compiler": "gcc", "compiler.version": "12", "compiler.libcxx": "libstdc++11"}
ARM = dict(LINUX, arch="armv8")
WIN = {"os": "Windows", "arch": "x86_64", "compiler": "msvc", "compiler.version": "194", "compiler.runtime": "dynamic"}

def cfg(base, bt, **extra):
    s = dict(base); s["build_type"] = bt
    if base["compiler"] == "msvc": s["compiler.runtime_type"] = bt
    s.update(extra); return s

C_LIB = lambda s: {k: v for k, v in s.items() if k not in ("compiler.libcxx", "compiler.cppstd")}

items, files = [], {}
day = [0]

def stamp():
    day[0] += 1
    d = day[0]
    return f"2026-{1 + d // 28:02d}-{1 + d % 28:02d}T{8 + d % 10:02d}:{(d * 7) % 60:02d}:13.{d * 37 % 1000:03d}+03:00"

def add(name, version, rrev_seed, settings, options, user="_", channel="_", props="full", prevs=1, uploader="ci-builder"):
    rrev = h(f"{name}/{version}/{rrev_seed}", 32)
    pkgid = h(f"{name}{version}{json.dumps(settings, sort_keys=True)}{json.dumps(options, sort_keys=True)}")
    for i in range(prevs):
        prev = h(f"{pkgid}{rrev}{i}", 32)
        path = f"{user}/{name}/{version}/{channel}/{rrev}/package/{pkgid}/{prev}"
        p = [{"key": "conan.package.name", "value": name}, {"key": "conan.package.version", "value": version},
             {"key": "conan.package.user", "value": user}, {"key": "conan.package.channel", "value": channel}]
        if props == "full":
            p += [{"key": "conan.settings." + k, "value": v} for k, v in sorted(settings.items())]
            p += [{"key": "conan.options." + k, "value": v} for k, v in sorted(options.items())]
        elif props == "none":
            p = []
        created = stamp()
        size = 40_000 + int(h(path, 6), 16) % 9_000_000
        items.append({"repo": REPO, "path": path, "name": "conan_package.tgz", "type": "file", "size": size,
                      "created": created, "created_by": uploader, "modified": created, "modified_by": uploader,
                      "properties": p})
        info = "[settings]\n" + "".join(f"{k}={v}\n" for k, v in sorted(settings.items()))
        info += "[options]\n" + "".join(f"{k}={v}\n" for k, v in sorted(options.items()))
        info += "[requires]\n"
        files[f"{REPO}/{path}/conaninfo.txt"] = info

lib = lambda shared: {"shared": shared, **({"fPIC": "True"} if shared == "False" else {})}
winlib = lambda shared: {"shared": shared}

# zlib 1.3.1, older recipe revision with two binaries
add("zlib", "1.3.1", "r1", C_LIB(cfg(LINUX, "Release")), lib("False"))
add("zlib", "1.3.1", "r1", C_LIB(cfg(WIN, "Release")), winlib("False"))
# zlib 1.3.1, latest recipe revision: full matrix
for bt in ("Release", "Debug"):
    for shared in ("False", "True"):
        add("zlib", "1.3.1", "r2", C_LIB(cfg(LINUX, bt)), lib(shared), prevs=2 if (bt, shared) == ("Release", "False") else 1)
        add("zlib", "1.3.1", "r2", C_LIB(cfg(WIN, bt)), winlib(shared), uploader="ayse.yilmaz")
add("zlib", "1.3.1", "r2", C_LIB(cfg(ARM, "Release")), lib("False"), uploader="arm-runner")
# zlib 1.2.13: Release only
add("zlib", "1.2.13", "r1", C_LIB(cfg(LINUX, "Release")), lib("False"))
add("zlib", "1.2.13", "r1", C_LIB(cfg(WIN, "Release")), winlib("False"))

# openssl 3.2.1: no armv8, no Windows Debug; Linux Debug has no properties (conaninfo fallback)
add("openssl", "3.2.1", "r1", C_LIB(cfg(LINUX, "Release")), lib("False"))
add("openssl", "3.2.1", "r1", C_LIB(cfg(LINUX, "Release")), lib("True"))
add("openssl", "3.2.1", "r1", C_LIB(cfg(LINUX, "Debug")), lib("False"), props="none")
add("openssl", "3.2.1", "r1", C_LIB(cfg(WIN, "Release")), winlib("False"), uploader="ayse.yilmaz")

# fmt 10.2.1@mycompany/stable: C++ library with cppstd
for bt in ("Release", "Debug"):
    add("fmt", "10.2.1", "r1", cfg(LINUX, bt, **{"compiler.cppstd": "17"}), lib("False"), user="mycompany", channel="stable")
    add("fmt", "10.2.1", "r1", cfg(WIN, bt, **{"compiler.cppstd": "17"}), winlib("False"), user="mycompany", channel="stable", uploader="ayse.yilmaz")
add("fmt", "10.2.1", "r1", cfg(ARM, "Release", **{"compiler.cppstd": "17"}), lib("False"), user="mycompany", channel="stable", uploader="arm-runner")
# fmt 11.0.2: only Linux so far
add("fmt", "11.0.2", "r1", cfg(LINUX, "Release", **{"compiler.cppstd": "17"}), lib("False"), user="mycompany", channel="stable")

# nlohmann_json 3.11.3: header-only, settings-free binary, name/version properties only
add("nlohmann_json", "3.11.3", "r1", {}, {}, props="ids")

doc = {"results": items, "range": {"start_pos": 0, "end_pos": len(items), "total": len(items)}}
with open(os.path.join(root, "aql.json"), "w") as f:
    json.dump(doc, f, indent=2); f.write("\n")
for rel, body in files.items():
    p = os.path.join(root, "files", rel)
    os.makedirs(os.path.dirname(p), exist_ok=True)
    with open(p, "w") as f: f.write(body)

profiles = {
    "linux-gcc12-x86_64-release": cfg(LINUX, "Release", **{"compiler.cppstd": "17"}),
    "linux-gcc12-x86_64-debug": cfg(LINUX, "Debug", **{"compiler.cppstd": "17"}),
    "linux-gcc12-armv8-release": cfg(ARM, "Release", **{"compiler.cppstd": "17"}),
    "win-msvc194-x64-release": cfg(WIN, "Release", **{"compiler.cppstd": "17"}),
    "win-msvc194-x64-debug": cfg(WIN, "Debug", **{"compiler.cppstd": "17"}),
}
pd = os.path.join(root, "profiles"); os.makedirs(pd, exist_ok=True)
for name, s in profiles.items():
    with open(os.path.join(pd, name), "w") as f:
        f.write("[settings]\n" + "".join(f"{k}={v}\n" for k, v in sorted(s.items())) + "\n[options]\n\n[conf]\n")
print(len(items), "items,", len(files), "conaninfo files,", len(profiles), "profiles")
