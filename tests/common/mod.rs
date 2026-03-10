use lastrun::model::Task;

pub fn make_task(id: &str) -> Task {
    Task {
        id: id.to_string(),
        last_run: None,
        start_time: None,
    }
}
