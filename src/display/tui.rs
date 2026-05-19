use crate::db;
use crate::error::AppResult;
use crate::format::format_duration;
use chrono::{DateTime, Duration, Local, Utc};
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
    widgets::{
        block::Title, Block, BorderType, Borders, Cell, Clear, Paragraph, Row, Table, TableState,
    },
    Frame, Terminal,
};
use rusqlite::Connection;
use std::io;
use std::time::{Duration as StdDuration, Instant};

#[derive(Clone, Copy, PartialEq)]
pub enum SortCol {
    Task,
    Status,
    Duration,
    Elapsed,
    LastRun,
}

struct TaskRow {
    id: String,
    last_run: Option<DateTime<Utc>>,
    start_time: Option<DateTime<Utc>>,
    duration: Option<i64>,
}

enum AppState {
    Normal,
    ConfirmDelete(String),
    History,
}

// Holds state for the history panel (shown when AppState::History)
struct HistoryView {
    task_id: String,
    // (raw_end_time_str, end_time, elapsed_ms) — raw string is used for deletion
    logs: Vec<(String, DateTime<Utc>, i64)>,
    table_state: TableState,
    confirm_delete: Option<String>, // raw end_time_str of the entry to delete
    page_size: usize,
}

impl HistoryView {
    fn load(conn: &Connection, task_id: String) -> AppResult<Self> {
        let logs = db::get_task_log_entries(conn, &task_id)?;
        let mut table_state = TableState::default();
        if !logs.is_empty() {
            table_state.select(Some(0));
        }
        Ok(Self { task_id, logs, table_state, confirm_delete: None, page_size: 10 })
    }

    fn refresh(&mut self, conn: &Connection) -> AppResult<()> {
        let selected_raw = self.selected_raw_end_time().map(|s| s.to_string());
        self.logs = db::get_task_log_entries(conn, &self.task_id)?;
        let new_idx = selected_raw.as_deref().and_then(|raw| {
            self.logs.iter().position(|(r, _, _)| r == raw)
        });
        self.table_state.select(new_idx.or_else(|| {
            if self.logs.is_empty() { None } else { Some(0) }
        }));
        Ok(())
    }

    fn nav_up(&mut self) {
        if self.logs.is_empty() {
            return;
        }
        let new_i = match self.table_state.selected() {
            Some(0) | None => self.logs.len() - 1,
            Some(i) => i - 1,
        };
        self.table_state.select(Some(new_i));
    }

    fn nav_down(&mut self) {
        if self.logs.is_empty() {
            return;
        }
        let new_i = match self.table_state.selected() {
            Some(i) if i + 1 < self.logs.len() => i + 1,
            _ => 0,
        };
        self.table_state.select(Some(new_i));
    }

    fn page_up(&mut self) {
        if self.logs.is_empty() {
            return;
        }
        let new_i = self.table_state.selected().unwrap_or(0).saturating_sub(self.page_size.max(1));
        self.table_state.select(Some(new_i));
    }

    fn page_down(&mut self) {
        if self.logs.is_empty() {
            return;
        }
        let new_i = (self.table_state.selected().unwrap_or(0) + self.page_size.max(1))
            .min(self.logs.len() - 1);
        self.table_state.select(Some(new_i));
    }

    fn selected_raw_end_time(&self) -> Option<&str> {
        self.table_state
            .selected()
            .and_then(|i| self.logs.get(i))
            .map(|(raw, _, _)| raw.as_str())
    }

    fn stats(&self) -> Option<(i64, i64, i64, Option<i64>)> {
        // (avg_ms, min_ms, max_ms, avg_frequency_ms)
        if self.logs.is_empty() {
            return None;
        }
        let n = self.logs.len();
        let sum: i64 = self.logs.iter().map(|(_, _, ms)| ms).sum();
        let avg = sum / n as i64;
        let min = self.logs.iter().map(|(_, _, ms)| *ms).min().unwrap();
        let max = self.logs.iter().map(|(_, _, ms)| *ms).max().unwrap();
        // Logs are ordered newest-first; span from oldest to newest divided by (n-1) gaps
        let avg_freq = if n >= 2 {
            let newest = self.logs[0].1;
            let oldest = self.logs[n - 1].1;
            let span_ms = newest.signed_duration_since(oldest).num_milliseconds();
            Some(span_ms / (n as i64 - 1))
        } else {
            None
        };
        Some((avg, min, max, avg_freq))
    }
}

struct App {
    tasks: Vec<TaskRow>,
    table_state: TableState,
    sort_col: SortCol,
    sort_asc: bool,
    app_state: AppState,
    last_updated: DateTime<Utc>,
    history_view: Option<HistoryView>,
    show_help: bool,
    page_size: usize,
}

impl App {
    fn new(
        tasks_raw: Vec<(String, Option<DateTime<Utc>>, Option<DateTime<Utc>>, Option<i64>)>,
        sort_col: SortCol,
    ) -> Self {
        let tasks: Vec<TaskRow> = tasks_raw
            .into_iter()
            .map(|(id, last_run, start_time, duration)| TaskRow {
                id,
                last_run,
                start_time,
                duration,
            })
            .collect();

        let mut app = Self {
            tasks,
            table_state: TableState::default(),
            sort_col,
            sort_asc: true,
            app_state: AppState::Normal,
            last_updated: Utc::now(),
            history_view: None,
            show_help: false,
            page_size: 10,
        };
        app.sort();
        if !app.tasks.is_empty() {
            app.table_state.select(Some(0));
        }
        app
    }

    fn refresh(&mut self, conn: &Connection) -> AppResult<()> {
        let selected_id = self
            .table_state
            .selected()
            .and_then(|i| self.tasks.get(i))
            .map(|t| t.id.clone());

        let tasks_raw = db::get_all_tasks(conn, None)?;
        self.tasks = tasks_raw
            .into_iter()
            .map(|(id, last_run, start_time, duration)| TaskRow {
                id,
                last_run,
                start_time,
                duration,
            })
            .collect();
        self.last_updated = Utc::now();
        self.sort();

        let new_idx = selected_id
            .as_ref()
            .and_then(|id| self.tasks.iter().position(|t| &t.id == id));
        self.table_state.select(new_idx.or_else(|| {
            if self.tasks.is_empty() { None } else { Some(0) }
        }));

        Ok(())
    }

    fn sort(&mut self) {
        let now = Utc::now();
        match self.sort_col {
            SortCol::Task => self.tasks.sort_by(|a, b| a.id.cmp(&b.id)),
            SortCol::Status => {
                self.tasks.sort_by_key(|t| task_status_order(t, &now));
            }
            SortCol::Duration => self.tasks.sort_by(|a, b| {
                match (a.duration, b.duration) {
                    (None, None) => std::cmp::Ordering::Equal,
                    (None, Some(_)) => std::cmp::Ordering::Greater,
                    (Some(_), None) => std::cmp::Ordering::Less,
                    (Some(x), Some(y)) => x.cmp(&y),
                }
            }),
            SortCol::Elapsed => {
                self.tasks.sort_by_key(|t| elapsed_millis(t, &now));
            }
            SortCol::LastRun => self.tasks.sort_by(|a, b| {
                match (a.last_run, b.last_run) {
                    (None, None) => std::cmp::Ordering::Equal,
                    (None, Some(_)) => std::cmp::Ordering::Greater,
                    (Some(_), None) => std::cmp::Ordering::Less,
                    (Some(x), Some(y)) => x.cmp(&y),
                }
            }),
        }
        if !self.sort_asc {
            self.tasks.reverse();
        }
    }

    fn toggle_sort_order(&mut self) {
        self.sort_asc = !self.sort_asc;
        self.sort();
    }

    fn nav_up(&mut self) {
        if self.tasks.is_empty() {
            return;
        }
        let new_i = match self.table_state.selected() {
            Some(0) | None => self.tasks.len() - 1,
            Some(i) => i - 1,
        };
        self.table_state.select(Some(new_i));
    }

    fn nav_down(&mut self) {
        if self.tasks.is_empty() {
            return;
        }
        let new_i = match self.table_state.selected() {
            Some(i) if i + 1 < self.tasks.len() => i + 1,
            _ => 0,
        };
        self.table_state.select(Some(new_i));
    }

    fn page_up(&mut self) {
        if self.tasks.is_empty() {
            return;
        }
        let new_i = self.table_state.selected().unwrap_or(0).saturating_sub(self.page_size.max(1));
        self.table_state.select(Some(new_i));
    }

    fn page_down(&mut self) {
        if self.tasks.is_empty() {
            return;
        }
        let new_i = (self.table_state.selected().unwrap_or(0) + self.page_size.max(1))
            .min(self.tasks.len() - 1);
        self.table_state.select(Some(new_i));
    }

    fn cycle_sort_next(&mut self) {
        self.sort_col = match self.sort_col {
            SortCol::Task => SortCol::Status,
            SortCol::Status => SortCol::Duration,
            SortCol::Duration => SortCol::Elapsed,
            SortCol::Elapsed => SortCol::LastRun,
            SortCol::LastRun => SortCol::Task,
        };
        self.sort();
    }

    fn cycle_sort_prev(&mut self) {
        self.sort_col = match self.sort_col {
            SortCol::Task => SortCol::LastRun,
            SortCol::Status => SortCol::Task,
            SortCol::Duration => SortCol::Status,
            SortCol::Elapsed => SortCol::Duration,
            SortCol::LastRun => SortCol::Elapsed,
        };
        self.sort();
    }

    fn selected_id(&self) -> Option<String> {
        self.table_state
            .selected()
            .and_then(|i| self.tasks.get(i))
            .map(|t| t.id.clone())
    }

    fn open_history(&mut self, conn: &Connection) -> AppResult<()> {
        if let Some(id) = self.selected_id() {
            self.history_view = Some(HistoryView::load(conn, id)?);
            self.app_state = AppState::History;
        }
        Ok(())
    }
}

fn task_status_order(task: &TaskRow, now: &DateTime<Utc>) -> u8 {
    if task.start_time.is_some() && task.last_run.is_none() {
        1
    } else if let Some(lr) = task.last_run {
        if let Some(d) = task.duration {
            if now.signed_duration_since(lr) > Duration::seconds(d) { 2 } else { 0 }
        } else {
            0
        }
    } else {
        3
    }
}

fn elapsed_millis(task: &TaskRow, now: &DateTime<Utc>) -> i64 {
    match (task.start_time, task.last_run) {
        (Some(st), Some(lr)) if st < lr => lr.signed_duration_since(st).num_milliseconds(),
        (Some(st), None) => now.signed_duration_since(st).num_milliseconds(),
        _ => 0,
    }
}

fn log_entry_color(ago: Duration) -> Color {
    let secs = ago.num_seconds();
    if secs < 3600 {
        Color::LightGreen
    } else if secs < 86400 {
        Color::Green
    } else if secs < 86400 * 7 {
        Color::Yellow
    } else {
        Color::Gray
    }
}

fn task_color(task: &TaskRow, now: &DateTime<Utc>) -> Color {
    if task.start_time.is_some() && task.last_run.is_none() {
        Color::Yellow
    } else if let Some(lr) = task.last_run {
        if let Some(d) = task.duration {
            if now.signed_duration_since(lr) > Duration::seconds(d) {
                Color::Red
            } else {
                Color::Green
            }
        } else {
            Color::White
        }
    } else {
        Color::Blue
    }
}

fn task_status_str(task: &TaskRow, now: &DateTime<Utc>) -> &'static str {
    if task.start_time.is_some() && task.last_run.is_none() {
        "running"
    } else if let Some(lr) = task.last_run {
        if let Some(d) = task.duration {
            if now.signed_duration_since(lr) > Duration::seconds(d) { "due" } else { "ok" }
        } else {
            "ok"
        }
    } else {
        "unknown"
    }
}

fn format_ago(ago: Duration) -> String {
    let secs = ago.num_seconds().abs();
    if secs < 60 {
        format!("{}s ago", secs)
    } else if secs < 3600 {
        let m = secs / 60;
        let s = secs % 60;
        if s == 0 { format!("{}m ago", m) } else { format!("{}m{}s ago", m, s) }
    } else if secs < 86400 {
        let h = secs / 3600;
        let m = (secs % 3600) / 60;
        if m == 0 { format!("{}h ago", h) } else { format!("{}h{}m ago", h, m) }
    } else {
        let d = secs / 86400;
        let h = (secs % 86400) / 3600;
        if h == 0 { format!("{}d ago", d) } else { format!("{}d{}h ago", d, h) }
    }
}


pub fn run_tui(conn: &Connection, id_filter: Option<String>, sort_col: SortCol) -> AppResult<()> {
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

    let result = run_app(&mut terminal, conn, id_filter, sort_col);

    disable_raw_mode()?;
    execute!(terminal.backend_mut(), LeaveAlternateScreen)?;
    terminal.show_cursor()?;

    result
}

fn run_app(
    terminal: &mut Terminal<CrosstermBackend<io::Stdout>>,
    conn: &Connection,
    id_filter: Option<String>,
    sort_col: SortCol,
) -> AppResult<()> {
    let tasks_raw = db::get_all_tasks(conn, id_filter)?;
    let mut app = App::new(tasks_raw, sort_col);
    let refresh_interval = StdDuration::from_secs(1);
    let mut last_refresh = Instant::now();

    loop {
        terminal.draw(|f| draw(f, &mut app))?;

        if event::poll(StdDuration::from_millis(250))? {
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

                let in_history = matches!(app.app_state, AppState::History);
                let confirming_log =
                    app.history_view.as_ref().map(|hv| hv.confirm_delete.is_some()).unwrap_or(false);

                if in_history {
                    if confirming_log {
                        match key.code {
                            KeyCode::Char('y') | KeyCode::Enter => {
                                if let Some(hv) = &mut app.history_view {
                                    if let Some(raw) = hv.confirm_delete.take() {
                                        let task_id = hv.task_id.clone();
                                        db::delete_task_log_entry(conn, &task_id, &raw)?;
                                        hv.refresh(conn)?;
                                    }
                                }
                            }
                            KeyCode::Char('n') | KeyCode::Esc | KeyCode::Char('q') => {
                                if let Some(hv) = &mut app.history_view {
                                    hv.confirm_delete = None;
                                }
                            }
                            _ => {}
                        }
                    } else {
                        match key.code {
                            KeyCode::Char('q') | KeyCode::Esc => {
                                app.app_state = AppState::Normal;
                                app.history_view = None;
                            }
                            KeyCode::Up | KeyCode::Char('k') => {
                                if let Some(hv) = &mut app.history_view {
                                    hv.nav_up();
                                }
                            }
                            KeyCode::Down | KeyCode::Char('j') => {
                                if let Some(hv) = &mut app.history_view {
                                    hv.nav_down();
                                }
                            }
                            KeyCode::PageUp => {
                                if let Some(hv) = &mut app.history_view {
                                    hv.page_up();
                                }
                            }
                            KeyCode::PageDown => {
                                if let Some(hv) = &mut app.history_view {
                                    hv.page_down();
                                }
                            }
                            KeyCode::Char('d') => {
                                if let Some(hv) = &mut app.history_view {
                                    if let Some(raw) = hv.selected_raw_end_time() {
                                        hv.confirm_delete = Some(raw.to_string());
                                    }
                                }
                            }
                            KeyCode::Char('r') => {
                                if let Some(hv) = &mut app.history_view {
                                    hv.refresh(conn)?;
                                }
                            }
                            KeyCode::Char('?') => {
                                app.show_help = true;
                            }
                            _ => {}
                        }
                    }
                } else if matches!(app.app_state, AppState::Normal) {
                    match key.code {
                        KeyCode::Char('q') | KeyCode::Esc => return Ok(()),
                        KeyCode::Up | KeyCode::Char('k') => app.nav_up(),
                        KeyCode::Down | KeyCode::Char('j') => app.nav_down(),
                        KeyCode::PageUp => app.page_up(),
                        KeyCode::PageDown => app.page_down(),
                        KeyCode::Right | KeyCode::Tab => app.cycle_sort_next(),
                        KeyCode::Left | KeyCode::BackTab => app.cycle_sort_prev(),
                        KeyCode::Char('s') => app.toggle_sort_order(),
                        KeyCode::Char('r') => {
                            app.refresh(conn)?;
                            last_refresh = Instant::now();
                        }
                        KeyCode::Char('d') => {
                            if let Some(id) = app.selected_id() {
                                app.app_state = AppState::ConfirmDelete(id);
                            }
                        }
                        KeyCode::Enter | KeyCode::Char('h') => {
                            app.open_history(conn)?;
                        }
                        KeyCode::Char('?') => {
                            app.show_help = true;
                        }
                        _ => {}
                    }
                } else {
                    // ConfirmDelete state
                    let task_id = if let AppState::ConfirmDelete(ref id) = app.app_state {
                        id.clone()
                    } else {
                        unreachable!()
                    };
                    match key.code {
                        KeyCode::Char('y') | KeyCode::Enter => {
                            db::delete_task_logs(conn, &task_id)?;
                            db::delete_task(conn, &task_id)?;
                            app.app_state = AppState::Normal;
                            app.refresh(conn)?;
                            last_refresh = Instant::now();
                        }
                        KeyCode::Char('n') | KeyCode::Esc | KeyCode::Char('q') => {
                            app.app_state = AppState::Normal;
                        }
                        _ => {}
                    }
                }
            }
        }

        if last_refresh.elapsed() >= refresh_interval {
            match app.app_state {
                AppState::Normal | AppState::ConfirmDelete(_) => {
                    app.refresh(conn)?;
                }
                AppState::History => {
                    if let Some(hv) = &mut app.history_view {
                        hv.refresh(conn)?;
                    }
                }
            }
            last_refresh = Instant::now();
        }
    }
}

fn draw(f: &mut Frame, app: &mut App) {
    let now = Utc::now();
    let in_history = matches!(app.app_state, AppState::History);
    let ctrl_h = controls_height(f.area().width, in_history);
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Min(0), Constraint::Length(ctrl_h)])
        .split(f.area());

    match app.app_state {
        AppState::Normal => {
            draw_table(f, app, chunks[0], &now);
            draw_controls(f, chunks[1], false);
        }
        AppState::ConfirmDelete(ref id) => {
            let id = id.clone();
            draw_table(f, app, chunks[0], &now);
            draw_controls(f, chunks[1], false);
            draw_confirm_task_popup(f, &id);
        }
        AppState::History => {
            if let Some(ref mut hv) = app.history_view {
                draw_history(f, hv, chunks[0], &now);
            }
            draw_controls(f, chunks[1], true);
            // Confirm-delete-log popup (rendered on top)
            if let Some(ref hv) = app.history_view {
                if let Some(ref raw) = hv.confirm_delete {
                    draw_confirm_log_popup(f, &hv.task_id, raw, &now);
                }
            }
        }
    }

    if app.show_help {
        let in_history = matches!(app.app_state, AppState::History);
        draw_help_modal(f, in_history);
    }
}

fn header_cell(label: &str, col: SortCol, active: SortCol, asc: bool) -> Cell<'static> {
    let text = if col == active {
        let indicator = if asc { "▲" } else { "▼" };
        format!("{} {}", label, indicator)
    } else {
        label.to_string()
    };
    let style = if col == active {
        Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)
    } else {
        Style::default().fg(Color::White).add_modifier(Modifier::BOLD)
    };
    Cell::from(text).style(style)
}

fn draw_table(f: &mut Frame, app: &mut App, area: Rect, now: &DateTime<Utc>) {
    let sc = app.sort_col;

    let right_pad = "─".repeat(if area.width < 60 { 3 } else if area.width < 100 { 2 } else { 1 });
    let updated = if area.width >= 50 {
        format!(" {}{}─", app.last_updated.with_timezone(&Local).format("%b %-d, %H:%M:%S "), right_pad)
    } else {
        format!(" {}{}─", app.last_updated.with_timezone(&Local).format("%H:%M:%S "), right_pad)
    };

    let block = Block::default()
        .borders(Borders::ALL)
        .border_type(BorderType::Rounded)
        .border_style(Style::default().fg(Color::DarkGray))
        .title(Title::from(Span::styled(
            " Last Run Status ",
            Style::default().fg(Color::White).add_modifier(Modifier::BOLD),
        )))
        .title(
            Title::from(Span::styled(updated, Style::default().fg(Color::White).add_modifier(Modifier::BOLD)))
                .alignment(Alignment::Right),
        );

    let asc = app.sort_asc;
    let header = Row::new(vec![
        header_cell("Task", SortCol::Task, sc, asc),
        header_cell("Status", SortCol::Status, sc, asc),
        header_cell("Duration", SortCol::Duration, sc, asc),
        header_cell("Elapsed", SortCol::Elapsed, sc, asc),
        header_cell("Last Run", SortCol::LastRun, sc, asc),
    ])
    .height(1)
    .bottom_margin(1);

    let rows: Vec<Row> = if app.tasks.is_empty() {
        vec![Row::new(vec![
            Cell::from("No tasks found").style(Style::default().fg(Color::DarkGray)),
        ])]
    } else {
        app.tasks
            .iter()
            .map(|task| {
                let color = task_color(task, now);
                let status = task_status_str(task, now);

                let duration_str = task
                    .duration
                    .map(|d| format_duration(Duration::seconds(d)))
                    .unwrap_or_else(|| "-".to_string());

                let elapsed_str = match (task.start_time, task.last_run) {
                    (Some(st), Some(lr)) if st < lr => {
                        format_duration(lr.signed_duration_since(st))
                    }
                    (Some(st), None) => format_duration(now.signed_duration_since(st)),
                    _ => "-".to_string(),
                };

                let last_run_str = task
                    .last_run
                    .map(|lr| format_duration(now.signed_duration_since(lr)))
                    .unwrap_or_else(|| "-".to_string());

                let style = Style::default().fg(color);
                Row::new(vec![
                    Cell::from(task.id.clone()).style(style),
                    Cell::from(status).style(style),
                    Cell::from(duration_str).style(style),
                    Cell::from(elapsed_str).style(style),
                    Cell::from(last_run_str).style(style),
                ])
            })
            .collect()
    };

    let widths = if area.width < 60 {
        [
            Constraint::Fill(1),
            Constraint::Length(8),
            Constraint::Length(7),
            Constraint::Length(7),
            Constraint::Length(7),
        ]
    } else {
        [
            Constraint::Fill(1),
            Constraint::Length(10),
            Constraint::Length(12),
            Constraint::Length(12),
            Constraint::Length(12),
        ]
    };

    let table = Table::new(rows, widths)
        .header(header)
        .block(block)
        .highlight_style(Style::default().add_modifier(Modifier::REVERSED))
        .highlight_symbol("▶ ");

    // 2 borders + 1 header row + 1 header margin
    app.page_size = (area.height.saturating_sub(4) as usize).max(1);
    f.render_stateful_widget(table, area, &mut app.table_state);
}

fn draw_history(f: &mut Frame, hv: &mut HistoryView, area: Rect, now: &DateTime<Utc>) {
    let total = hv.logs.len();
    let right_pad = "─".repeat(if area.width < 60 { 3 } else if area.width < 100 { 2 } else { 1 });
    let title_right = format!(" {} run{} {}", total, if total == 1 { "" } else { "s" }, right_pad);

    let block = Block::default()
        .borders(Borders::ALL)
        .border_type(BorderType::Rounded)
        .border_style(Style::default().fg(Color::DarkGray))
        .title(Title::from(Span::styled(
            format!(" Task History: {} ", hv.task_id),
            Style::default().fg(Color::White).add_modifier(Modifier::BOLD),
        )))
        .title(
            Title::from(Span::styled(title_right, Style::default().fg(Color::White).add_modifier(Modifier::BOLD)))
                .alignment(Alignment::Right),
        );

    let inner = block.inner(area);
    f.render_widget(block, area);

    // Split inner: stats bar (2 lines) + table
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(2), Constraint::Min(0)])
        .split(inner);

    // ── Stats ──
    let stats_line = if let Some((avg, min, max, avg_freq)) = hv.stats() {
        let avg_str = format_duration(Duration::milliseconds(avg));
        let min_str = format_duration(Duration::milliseconds(min));
        let max_str = format_duration(Duration::milliseconds(max));
        let mut spans = vec![
            Span::styled("  Avg: ", Style::default().fg(Color::DarkGray)),
            Span::styled(avg_str, Style::default().fg(Color::White).add_modifier(Modifier::BOLD)),
            Span::styled("   Min: ", Style::default().fg(Color::DarkGray)),
            Span::styled(min_str, Style::default().fg(Color::Green)),
            Span::styled("   Max: ", Style::default().fg(Color::DarkGray)),
            Span::styled(max_str, Style::default().fg(Color::Yellow)),
        ];
        if let Some(freq_ms) = avg_freq {
            let freq_str = format_duration(Duration::milliseconds(freq_ms));
            spans.push(Span::styled("   Freq: every ~", Style::default().fg(Color::DarkGray)));
            spans.push(Span::styled(freq_str, Style::default().fg(Color::Cyan)));
        }
        Line::from(spans)
    } else {
        Line::from(Span::styled("  No run history recorded.", Style::default().fg(Color::DarkGray)))
    };
    f.render_widget(Paragraph::new(vec![stats_line, Line::from("")]), chunks[0]);

    // ── History table ──
    let header_style = Style::default().fg(Color::DarkGray).add_modifier(Modifier::BOLD);
    let header = Row::new(vec![
        Cell::from("#").style(header_style),
        Cell::from("Completed At").style(header_style),
        Cell::from("Duration").style(header_style),
        Cell::from("Time Ago").style(header_style),
    ])
    .height(1)
    .bottom_margin(0);

    let rows: Vec<Row> = if hv.logs.is_empty() {
        vec![Row::new(vec![
            Cell::from(""),
            Cell::from("No log entries found.").style(Style::default().fg(Color::DarkGray)),
        ])]
    } else {
        hv.logs
            .iter()
            .enumerate()
            .map(|(i, (_, end_time, elapsed_ms))| {
                let ago = now.signed_duration_since(*end_time);
                Row::new(vec![
                    Cell::from(format!("{:>3}", i + 1)).style(Style::default().fg(Color::DarkGray)),
                    Cell::from(
                        end_time.with_timezone(&Local).format("%Y-%m-%d %H:%M:%S").to_string(),
                    )
                    .style(Style::default().fg(log_entry_color(ago)).add_modifier(Modifier::BOLD)),
                    Cell::from(format_duration(Duration::milliseconds(*elapsed_ms)))
                        .style(Style::default().fg(Color::White)),
                    Cell::from(format_ago(ago)).style(Style::default().fg(Color::DarkGray)),
                ])
            })
            .collect()
    };

    let widths = [
        Constraint::Length(4),
        Constraint::Length(21),
        Constraint::Length(12),
        Constraint::Fill(1),
    ];

    let table = Table::new(rows, widths)
        .header(header)
        .highlight_style(Style::default().add_modifier(Modifier::REVERSED))
        .highlight_symbol("▶ ");

    // 2 borders (from outer block) + 2 stats lines + 1 header row
    hv.page_size = (chunks[1].height.saturating_sub(1) as usize).max(1);
    f.render_stateful_widget(table, chunks[1], &mut hv.table_state);
}

fn get_basic_shortcuts(in_history: bool) -> &'static [(&'static str, &'static str)] {
    if in_history {
        &[("↑↓/jk", "Navigate"), ("?", "All Keys"), ("q/Esc", "Back")]
    } else {
        &[
            ("↑↓/jk", "Navigate"),
            ("Enter/h", "History"),
            ("?", "All Keys"),
            ("q/Esc", "Quit"),
        ]
    }
}

fn get_all_shortcuts(in_history: bool) -> &'static [(&'static str, &'static str)] {
    if in_history {
        &[
            ("↑↓/jk", "Navigate"),
            ("PgUp/PgDn", "Page"),
            ("d", "Delete Entry"),
            ("r", "Refresh"),
            ("?", "Help"),
            ("q/Esc", "Back"),
        ]
    } else {
        &[
            ("↑↓/jk", "Navigate"),
            ("PgUp/PgDn", "Page"),
            ("←→/Tab", "Sort"),
            ("s", "Toggle Order"),
            ("Enter/h", "History"),
            ("d", "Delete Task"),
            ("r", "Refresh"),
            ("?", "Help"),
            ("q/Esc", "Quit"),
        ]
    }
}

fn controls_height(terminal_width: u16, in_history: bool) -> u16 {
    let shortcuts = get_basic_shortcuts(in_history);
    let content_width = terminal_width.saturating_sub(2) as usize;
    let mut lines = 1u16;
    let mut current_width = 2usize; // leading spaces
    let mut first_on_line = true;

    for (key, desc) in shortcuts.iter() {
        let sep = if first_on_line { 0 } else { 2 };
        let item_width = sep + key.chars().count() + 2 + 1 + desc.chars().count();
        if !first_on_line && current_width + item_width > content_width {
            lines += 1;
            current_width = 2 + key.chars().count() + 2 + 1 + desc.chars().count();
            first_on_line = true;
        } else {
            current_width += item_width;
            first_on_line = false;
        }
    }

    lines + 2 // +2 for borders
}

fn draw_controls(f: &mut Frame, area: Rect, in_history: bool) {
    let key_style =
        Style::default().fg(Color::Black).bg(Color::DarkGray).add_modifier(Modifier::BOLD);
    let desc_style = Style::default().fg(Color::Gray);
    let sep_style = Style::default().fg(Color::DarkGray);

    let shortcuts = get_basic_shortcuts(in_history);
    let content_width = area.width.saturating_sub(2) as usize;

    let mut lines: Vec<Line> = vec![];
    let mut current_spans: Vec<Span> = vec![Span::raw("  ")];
    let mut current_width = 2usize;
    let mut first_on_line = true;

    for (key, desc) in shortcuts.iter() {
        let sep = if first_on_line { 0 } else { 2 };
        let item_width = sep + key.chars().count() + 2 + 1 + desc.chars().count();
        if !first_on_line && current_width + item_width > content_width {
            lines.push(Line::from(std::mem::take(&mut current_spans)));
            current_spans = vec![Span::raw("  ")];
            current_width = 2;
            first_on_line = true;
        }
        if !first_on_line {
            current_spans.push(Span::styled("  ", sep_style));
            current_width += 2;
        }
        current_spans.push(Span::styled(format!(" {} ", key), key_style));
        current_spans.push(Span::raw(" "));
        current_spans.push(Span::styled(*desc, desc_style));
        current_width += key.chars().count() + 2 + 1 + desc.chars().count();
        first_on_line = false;
    }
    if !current_spans.is_empty() {
        lines.push(Line::from(current_spans));
    }

    let block = Block::default()
        .borders(Borders::ALL)
        .border_type(BorderType::Rounded)
        .border_style(Style::default().fg(Color::DarkGray))
        .title(Span::styled(" Keys ", Style::default().fg(Color::White)));

    f.render_widget(Paragraph::new(lines).block(block), area);
}

fn draw_help_modal(f: &mut Frame, in_history: bool) {
    let shortcuts = get_all_shortcuts(in_history);
    let key_style =
        Style::default().fg(Color::Black).bg(Color::DarkGray).add_modifier(Modifier::BOLD);
    let desc_style = Style::default().fg(Color::Gray);

    let max_key_len = shortcuts.iter().map(|(k, _)| k.chars().count() + 2).max().unwrap_or(4);
    let max_desc_len = shortcuts.iter().map(|(_, d)| d.chars().count()).max().unwrap_or(4);
    let inner_width = (max_key_len + 1 + max_desc_len + 4) as u16;
    let area = f.area();
    let popup_width = (inner_width + 2).min(area.width.saturating_sub(4)).max(30);
    let popup_height = (shortcuts.len() as u16 + 2).min(area.height.saturating_sub(2));
    let x = area.x + (area.width.saturating_sub(popup_width)) / 2;
    let y = area.y + (area.height.saturating_sub(popup_height)) / 2;
    let popup_area = Rect::new(x, y, popup_width, popup_height);

    let lines: Vec<Line> = shortcuts
        .iter()
        .map(|(key, desc)| {
            Line::from(vec![
                Span::raw("  "),
                Span::styled(format!(" {} ", key), key_style),
                Span::raw(" "),
                Span::styled(*desc, desc_style),
            ])
        })
        .collect();

    f.render_widget(Clear, popup_area);
    f.render_widget(
        Paragraph::new(lines).block(
            Block::default()
                .borders(Borders::ALL)
                .border_type(BorderType::Rounded)
                .border_style(Style::default().fg(Color::White))
                .title(Span::styled(
                    " All Keys ",
                    Style::default().fg(Color::White).add_modifier(Modifier::BOLD),
                ))
                .title(
                    Title::from(Span::styled(
                        " ? or Esc to close ",
                        Style::default().fg(Color::DarkGray),
                    ))
                    .alignment(Alignment::Right),
                ),
        ),
        popup_area,
    );
}

fn draw_confirm_task_popup(f: &mut Frame, task_id: &str) {
    let area = f.area();
    let content = format!(" Delete \"{}\" and all its history?  [y] Yes   [n] No ", task_id);
    let popup_width = (content.len() as u16 + 4).min(area.width.saturating_sub(4)).max(30);
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
                    .border_style(Style::default().fg(Color::Red))
                    .title(Span::styled(
                        " Confirm Delete Task ",
                        Style::default().fg(Color::Red).add_modifier(Modifier::BOLD),
                    )),
            )
            .alignment(Alignment::Center),
        popup_area,
    );
}

fn draw_confirm_log_popup(f: &mut Frame, task_id: &str, raw_end_time: &str, now: &DateTime<Utc>) {
    // Parse the raw string for display; fall back to the raw string if it fails
    let display_time = DateTime::parse_from_rfc3339(raw_end_time)
        .ok()
        .map(|dt| {
            let ago = now.signed_duration_since(dt.with_timezone(&Utc));
            format!(
                "{} ({})",
                dt.with_timezone(&Local).format("%Y-%m-%d %H:%M:%S"),
                format_ago(ago),
            )
        })
        .unwrap_or_else(|| raw_end_time.to_string());

    let area = f.area();
    let content = format!(" Delete entry from {}?  [y] Yes   [n] No ", display_time);
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
                    .border_style(Style::default().fg(Color::Red))
                    .title(Span::styled(
                        format!(" Delete Log Entry — {} ", task_id),
                        Style::default().fg(Color::Red).add_modifier(Modifier::BOLD),
                    )),
            )
            .alignment(Alignment::Center),
        popup_area,
    );
}
