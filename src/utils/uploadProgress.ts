const filePosMap = new Map<number, number>()
let uploadSpeedTotal = 0

export const getFileUploadSpeed = (uploadId: number): number => filePosMap.get(uploadId) || 0

export const recordUploadProgress = (uploadId: number, delta: number, pos: number): void => {
  if (delta > 0) uploadSpeedTotal += delta
  filePosMap.set(uploadId, pos)
}

export const delFileUploadSpeed = (uploadId: number): void => {
  filePosMap.delete(uploadId)
}

export const getFileUploadSpeedTotal = (): number => {
  const speed = Number(uploadSpeedTotal)
  uploadSpeedTotal = 0
  return speed
}
