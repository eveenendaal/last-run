fn main() {
    if let Ok(v) = std::env::var("RELEASE_VERSION") {
        if !v.is_empty() {
            println!("cargo:rustc-env=APP_VERSION={}", v.trim());
            return;
        }
    }
    let output = std::process::Command::new("git")
        .args(["describe", "--tags", "--abbrev=0"])
        .output();
    if let Ok(out) = output {
        if out.status.success() {
            let tag = String::from_utf8_lossy(&out.stdout);
            let version = tag.trim().trim_start_matches('v');
            println!("cargo:rustc-env=APP_VERSION={}", version);
            return;
        }
    }
    println!("cargo:rustc-env=APP_VERSION={}", env!("CARGO_PKG_VERSION"));
}
