//! Runs sonic-pi-lsp for Ruby buffers in Zed.
//!
//! The server comes from `lsp.sonic-pi-lsp.binary` in Zed's settings, else
//! from `sonic-pi-lsp` on the PATH (`npm install -g` from the repository).

use zed_extension_api::{self as zed, settings::LspSettings, LanguageServerId, Result};

const SERVER: &str = "sonic-pi-lsp";

struct SonicPiExtension;

impl zed::Extension for SonicPiExtension {
    fn new() -> Self {
        SonicPiExtension
    }

    fn language_server_command(
        &mut self,
        _language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        let binary = LspSettings::for_worktree(SERVER, worktree)
            .ok()
            .and_then(|settings| settings.binary);
        let args = binary
            .as_ref()
            .and_then(|b| b.arguments.clone())
            .unwrap_or_else(|| vec!["--stdio".into()]);

        if let Some(path) = binary.and_then(|b| b.path) {
            return Ok(zed::Command { command: path, args, env: worktree.shell_env() });
        }
        let path = worktree.which(SERVER).ok_or_else(|| {
            format!(
                "{SERVER} is not on your PATH. Install it (see https://github.com/AlexDev404/sonic-pi-lsp) \
                 or set lsp.{SERVER}.binary.path in your Zed settings."
            )
        })?;
        Ok(zed::Command { command: path, args, env: worktree.shell_env() })
    }
}

zed::register_extension!(SonicPiExtension);
