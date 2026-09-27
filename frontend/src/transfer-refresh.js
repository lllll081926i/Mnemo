export function mergeLiveTasks(snapshot, events, keyOf) {
  const tasks = Array.isArray(snapshot) ? [...snapshot] : []
  const positions = new Map(tasks.map((task, index) => [keyOf(task), index]))
  const added = []
  for (const [id, task] of events) {
    const index = positions.get(id)
    if (index === undefined) {
      if (task) added.unshift(task)
    } else {
      tasks[index] = task
    }
  }
  return [...added, ...tasks.filter(Boolean)]
}
