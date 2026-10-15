export function summarizeSync(changes = []) {
  const counts = { upload: 0, download: 0, conflict: 0, 'delete-local': 0, 'delete-remote': 0 }
  for (const change of changes) if (Object.hasOwn(counts, change.action)) counts[change.action]++
  return { ...counts, deletes: counts['delete-local'] + counts['delete-remote'] }
}
