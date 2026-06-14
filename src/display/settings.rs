use super::tui::{controls_height, draw_controls, draw_help_modal};
use crate::db;
use crate::error::AppResult;
use crossterm::{
    event::{self, Event, KeyCode},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use ratatui::{
    backend::CrosstermBackend,
    layout::{Alignment, Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, BorderType, Borders, Cell, Clear, Paragraph, Row, Table},
    Frame, Terminal,
};
use rusqlite::Connection;
use std::io;

enum SettingsState {
    List,
    Edit(String, String), // (key, current input buffer)
}

struct SettingsApp {
    settings: Vec<(String, String)>,
    state: SettingsState,
    show_help: bool,
}

impl SettingsApp {
    fn new(conn: &Connection) -> Self {
        let mut app = Self { settings: Vec::new(), state: SettingsState::List, show_help: false };
        app.reload(conn);
        app
    }

    fn reload(&mut self, conn: &Connection) {
        let mut settings = db::get_all_settings(conn).unwrap_or_default();
        // Always surface `log_retention` so it can be edited even on a fresh DB
        // where no setting has been written yet. The DB stays untouched (unset
        // still means "use the 30d default"); this row is display-only until saved.
        if !settings.iter().any(|(k, _)| k == "log_retention") {
            settings.push(("log_retention".to_string(), "30d".to_string()));
            settings.sort();
        }
        self.settings = settings;
    }
}

pub fn run_settings_tui(conn: &Connection) -> AppResult<()> {
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen)?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    let original_hook = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        let _ = disable_raw_mode();
        let _ = execute!(io::stdout(), LeaveAlternateScreen);
        original_hook(info);
    }));

    let result = run_app(&mut terminal, conn);

    disable_raw_mode()?;
    execute!(terminal.backend_mut(), LeaveAlternateScreen)?;
    terminal.show_cursor()?;

    result
}

fn run_app(terminal: &mut Terminal<CrosstermBackend<io::Stdout>>, conn: &Connection) -> AppResult<()> {
    let mut app = SettingsApp::new(conn);

    loop {
        terminal.draw(|f| draw(f, &app))?;

        if let Event::Key(key) = event::read()? {
            // Help modal intercepts all keys
            if app.show_help {
                match key.code {
                    KeyCode::Char('?') | KeyCode::Esc | KeyCode::Char('q') => {
                        app.show_help = false;
                    }
                    _ => {}
                }
                continue;
            }

            match &app.state {
                SettingsState::List => match key.code {
                    KeyCode::Char('q') | KeyCode::Esc => return Ok(()),
                    KeyCode::Enter => {
                        if let Some((key, value)) = app.settings.first() {
                            app.state = SettingsState::Edit(key.clone(), value.clone());
                        }
                    }
                    KeyCode::Char('?') => {
                        app.show_help = true;
                    }
                    _ => {}
                },
                SettingsState::Edit(_, _) => {
                    let (setting_key, mut buf) = match &app.state {
                        SettingsState::Edit(k, v) => (k.clone(), v.clone()),
                        _ => unreachable!(),
                    };
                    match key.code {
                        KeyCode::Enter => {
                            // Route `log_retention` through the validating setter so a
                            // typo can't be stored and then silently ignored. On a
                            // validation error, stay in the editor instead of saving.
                            let saved = if setting_key == "log_retention" {
                                db::set_log_retention(conn, buf.trim()).is_ok()
                            } else {
                                db::set_setting(conn, &setting_key, &buf).is_ok()
                            };
                            if saved {
                                app.reload(conn);
                                app.state = SettingsState::List;
                            } else {
                                app.state = SettingsState::Edit(setting_key, buf);
                            }
                        }
                        KeyCode::Esc => {
                            app.state = SettingsState::List;
                        }
                        KeyCode::Backspace => {
                            buf.pop();
                            app.state = SettingsState::Edit(setting_key, buf);
                        }
                        KeyCode::Char(c) => {
                            buf.push(c);
                            app.state = SettingsState::Edit(setting_key, buf);
                        }
                        _ => {}
                    }
                }
            }
        }
    }
}

fn basic_shortcuts(state: &SettingsState) -> &'static [(&'static str, &'static str)] {
    match state {
        SettingsState::List => &[("Enter", "Edit"), ("?", "All Keys"), ("q/Esc", "Quit")],
        SettingsState::Edit(_, _) => &[("Enter", "Save"), ("Esc", "Cancel")],
    }
}

fn all_shortcuts(state: &SettingsState) -> &'static [(&'static str, &'static str)] {
    match state {
        SettingsState::List => &[("Enter", "Edit Setting"), ("?", "Help"), ("q/Esc", "Quit")],
        SettingsState::Edit(_, _) => &[
            ("Enter", "Save"),
            ("Esc", "Cancel"),
            ("Backspace", "Delete"),
        ],
    }
}

fn draw(f: &mut Frame, app: &SettingsApp) {
    let shortcuts = basic_shortcuts(&app.state);
    let ctrl_h = controls_height(f.area().width, shortcuts);
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Min(0), Constraint::Length(ctrl_h)])
        .split(f.area());

    draw_settings_table(f, app, chunks[0]);
    draw_controls(f, chunks[1], shortcuts);

    if let SettingsState::Edit(ref key, ref value) = app.state {
        draw_edit_setting_popup(f, key, value);
    }

    if app.show_help {
        draw_help_modal(f, all_shortcuts(&app.state));
    }
}

fn draw_settings_table(f: &mut Frame, app: &SettingsApp, area: Rect) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_type(BorderType::Rounded)
        .border_style(Style::default().fg(Color::DarkGray))
        .title(Span::styled(
            " Settings ",
            Style::default().fg(Color::White).add_modifier(Modifier::BOLD),
        ));

    let header = Row::new(vec![
        Cell::from("Key").style(Style::default().fg(Color::DarkGray).add_modifier(Modifier::BOLD)),
        Cell::from("Value").style(Style::default().fg(Color::DarkGray).add_modifier(Modifier::BOLD)),
    ])
    .height(1)
    .bottom_margin(1);

    let rows: Vec<Row> = if app.settings.is_empty() {
        vec![Row::new(vec![
            Cell::from("No settings configured.").style(Style::default().fg(Color::DarkGray)),
        ])]
    } else {
        app.settings
            .iter()
            .map(|(key, value)| {
                Row::new(vec![
                    Cell::from(key.clone()).style(Style::default().fg(Color::White)),
                    Cell::from(value.clone()).style(Style::default().fg(Color::Yellow)),
                ])
            })
            .collect()
    };

    let widths = [Constraint::Fill(1), Constraint::Fill(1)];

    let table = Table::new(rows, widths).header(header).block(block);

    f.render_widget(table, area);
}

fn draw_edit_setting_popup(f: &mut Frame, key: &str, value: &str) {
    let area = f.area();
    let content = format!(" Edit {}: {}█ ", key, value);
    let popup_width = (content.len() as u16 + 4).min(area.width.saturating_sub(4)).max(40);
    let popup_height = 3u16;
    let x = area.x + (area.width.saturating_sub(popup_width)) / 2;
    let y = area.y + (area.height.saturating_sub(popup_height)) / 2;
    let popup_area = Rect::new(x, y, popup_width, popup_height);

    f.render_widget(Clear, popup_area);
    f.render_widget(
        Paragraph::new(content)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .border_type(BorderType::Rounded)
                    .border_style(Style::default().fg(Color::Yellow))
                    .title(Span::styled(
                        format!(" Edit Setting — {} ", key),
                        Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD),
                    ))
                    .title(
                        Line::from(Span::styled(
                            " Enter to save  Esc to cancel ",
                            Style::default().fg(Color::DarkGray),
                        ))
                        .right_aligned(),
                    ),
            )
            .alignment(Alignment::Left),
        popup_area,
    );
}
