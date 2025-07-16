use std::process::Command;
use std::fs;

#[test]
fn test_version_updating() {
    // Test the version updating logic used in the GitHub Actions workflow
    
    // Create a temporary Cargo.toml for testing
    let temp_cargo = "temp_Cargo.toml";
    let original_content = r#"[package]
name = "lastrun"
version = "0.2.0"
edition = "2021"
authors = ["Eric Veenendaal"]
description = "A utility to track when tasks were last run"

[dependencies]
rusqlite = "0.37.0"
chrono = "0.4.26"
clap = { version = "4.5.41", features = ["derive"] }
dirs = "6.0.0"
thiserror = "2.0.12"
prettytable-rs = "0.10.0"
clap_complete = "4.5"
serde_json = "1.0"
"#;
    
    fs::write(temp_cargo, original_content).unwrap();
    
    // Test version replacement
    let output = Command::new("sed")
        .arg("-i")
        .arg("s/^version = \".*\"/version = \"1.0.0\"/")
        .arg(temp_cargo)
        .output()
        .expect("Failed to execute sed command");
    
    assert!(output.status.success(), "sed command failed: {:?}", output);
    
    // Verify the version was updated
    let updated_content = fs::read_to_string(temp_cargo).unwrap();
    assert!(updated_content.contains("version = \"1.0.0\""), "Version was not updated correctly");
    
    // Clean up
    fs::remove_file(temp_cargo).unwrap();
}

#[test]
fn test_version_increment_logic() {
    // Test the version increment logic
    
    // Test starting from no version tags (should be 1.0.0)
    let new_version = get_next_version(None);
    assert_eq!(new_version, "1.0.0");
    
    // Test incrementing from v1.0.0 -> 1.1.0
    let new_version = get_next_version(Some("v1.0.0"));
    assert_eq!(new_version, "1.1.0");
    
    // Test incrementing from v1.5.0 -> 1.6.0
    let new_version = get_next_version(Some("v1.5.0"));
    assert_eq!(new_version, "1.6.0");
    
    // Test incrementing from v2.3.0 -> 2.4.0
    let new_version = get_next_version(Some("v2.3.0"));
    assert_eq!(new_version, "2.4.0");
}

// Helper function that mimics the logic from GitHub Actions workflow
fn get_next_version(latest_tag: Option<&str>) -> String {
    match latest_tag {
        None => "1.0.0".to_string(),
        Some(tag) => {
            // Remove the 'v' prefix
            let current_version = tag.trim_start_matches('v');
            let parts: Vec<&str> = current_version.split('.').collect();
            
            if parts.len() == 3 {
                let major: i32 = parts[0].parse().unwrap_or(1);
                let minor: i32 = parts[1].parse().unwrap_or(0);
                let _patch: i32 = parts[2].parse().unwrap_or(0);
                
                let new_minor = minor + 1;
                format!("{}.{}.0", major, new_minor)
            } else {
                "1.0.0".to_string()
            }
        }
    }
}