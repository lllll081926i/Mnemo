export interface BuildAriaAddOptionsInput {
  gid: string
  dir: string
  split: number
  referer: string
  userAgent: string
  headers: string[]
  outFileName: string
  allProxy?: string
}

export const buildAriaAddOptions = (input: BuildAriaAddOptionsInput): Record<string, any> => {
  const options: Record<string, any> = {
    gid: input.gid,
    dir: input.dir,
    split: input.split,
    out: input.outFileName
  }

  if (input.referer) options.referer = input.referer
  if (input.userAgent) options['user-agent'] = input.userAgent
  if (input.headers.length) options.header = input.headers

  if (input.allProxy) options['all-proxy'] = input.allProxy
  return options
}
