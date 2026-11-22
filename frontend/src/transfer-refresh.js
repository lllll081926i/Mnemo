function samePayload(left, right) {
  if (Object.is(left, right)) return true
  if (!left || !right || typeof left !== 'object' || typeof right !== 'object') return false
  if (Array.isArray(left) !== Array.isArray(right)) return false
  if (Array.isArray(left) && left.length !== right.length) return false
  const keys = Object.keys(left)
  return keys.length === Object.keys(right).length && keys.every(key => Object.hasOwn(right, key) && samePayload(left[key], right[key]))
}

export function mergeLiveTasks(snapshot, events, keyOf, previous = []) {
  const tasks = Array.isArray(snapshot) ? [...snapshot] : []
  const positions = events.size ? new Map(tasks.map((task, index) => [keyOf(task), index])) : null
  const added = []
  for (const [id, task] of events) {
    const index = positions.get(id)
    if (index === undefined) {
      if (task) added.push(task)
    } else {
      tasks[index] = task
    }
  }
  const next = [...added.reverse(), ...tasks.filter(Boolean)]
  let previousByID
  let unchanged = next.length === previous.length
  for (let index = 0; index < next.length; index++) {
    const id = keyOf(next[index])
    let old = previous[index]
    if (!old || keyOf(old) !== id) {
      previousByID ??= new Map(previous.map(task => [keyOf(task), task]))
      old = previousByID.get(id)
    }
    if (old && samePayload(old, next[index])) next[index] = old
    if (next[index] !== previous[index]) unchanged = false
  }
  return unchanged ? previous : next
}
