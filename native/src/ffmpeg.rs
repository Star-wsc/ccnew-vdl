use std::path::Path;
use std::process::Command;

pub fn merge_audio_video(video: &Path, audio: &Path, output: &Path) -> anyhow::Result<()> {
    let status = Command::new("ffmpeg")
        .args(["-i", video.to_str().unwrap_or_default()])
        .args(["-i", audio.to_str().unwrap_or_default()])
        .args(["-c:v", "copy", "-c:a", "copy", "-y"])
        .arg(output.to_str().unwrap_or_default())
        .status()?;
    if status.success() {
        Ok(())
    } else {
        Err(anyhow::anyhow!("ffmpeg 合并失败: {}", status))
    }
}