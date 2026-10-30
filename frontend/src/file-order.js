const collator = new Intl.Collator('zh-Hans-CN', { numeric: true, sensitivity: 'base' })

export function compareFileNames(a, b) {
  return collator.compare(a.name || '', b.name || '') || String(a.name || '').localeCompare(String(b.name || '')) || String(a.file_id || '').localeCompare(String(b.file_id || ''))
}

export function sortedDirectories(files) {
  return files.filter(file => file.isDir).sort(compareFileNames)
}
