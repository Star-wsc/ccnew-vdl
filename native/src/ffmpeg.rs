use std::path::Path;

pub fn merge_audio_video(video: &Path, audio: &Path, output: &Path) -> anyhow::Result<()> {
    let mut cmd = std::process::Command::new("ffmpeg");
    cmd.args(["-i", video.to_str().unwrap_or_default()])
        .args(["-i", audio.to_str().unwrap_or_default()])
        .args(["-c:v", "copy", "-c:a", "copy", "-y"])
        .arg(output.to_str().unwrap_or_default());

    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        cmd.creation_flags(0x08000000); // CREATE_NO_WINDOW
    }

    let status = cmd.status()?;
    if status.success() { Ok(()) } else { Err(anyhow::anyhow!("ffmpeg 合并失败: {}", status)) }
}
