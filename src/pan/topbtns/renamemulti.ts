export interface TreeNodeData {
  key: string
  title: string
  rawtitle: string
  newtitle: string
  isLeaf: boolean
  children: TreeNodeData[]
  icon: any
  isDir: boolean
  
  isMatch: boolean
}

const escapeHtml = (str: string): string => str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')

export function NewRenameConfigData() {
  return {
    
    show: false,
    
    replace: { enable: true, search: '', newword: '', chkCase: true, chkAll: true, chkReg: false, applyto: 'name' },
    
    delete: { enable: false, type: 'search', search: '', chkCase: true, chkAll: true, chkReg: false, applyto: 'name', beginlen: 0, endlen: 0, beginword: '', endword: '' },
    
    add: { enable: false, type: 'position', search: '', before: '', after: '', beginword: '', endword: '', applyto: 'name' },
    
    index: { enable: false, type: 'begin', format: '', minlen: 1, beginindex: 1, minnum: 1 },
    
    others: { enable: false, nameformat: '', extformat: '', randomformat: '', randomlen: 4 }
  }
}


export function RunAllNode(nodeList: TreeNodeData[], func: (node: TreeNodeData) => boolean) {
  for (let i = 0, maxi = nodeList.length; i < maxi; i++) {
    const node = nodeList[i]
    if (!func(node)) return false 

    if (node.children && node.children.length > 0) {
      if (!RunAllNode(node.children, func)) return false 
    }
  }
  return true
}


function Split(text: string, search: string) {
  const textLow = text.toLowerCase()
  search = search.toLowerCase()
  const listLow = textLow.split(search)
  const searchLen = search.length
  let pos = 0
  const list: string[] = []
  for (let i = 0, maxi = listLow.length; i < maxi; i++) {
    const low = listLow[i]
    if (low) {
      list.push(text.substring(pos, pos + low.length))
      pos = pos + searchLen + low.length
    } else {
      list.push('')
      pos = pos + searchLen
    }
  }
  return list
}

function Replace(text: string, search: string, newtext: string) {
  const textLow = text.toLowerCase()
  search = search.toLowerCase()
  const index = textLow.indexOf(search)
  if (index >= 0) {
    return text.substring(0, index) + newtext + text.substring(index + search.length)
  } else return text
}

// 与 Replace 相同的定位逻辑，但用于生成高亮 HTML：原始文件名片段会被转义，仅保留插入的高亮标签
function ReplaceHtml(text: string, search: string, newtext: string) {
  const textLow = text.toLowerCase()
  search = search.toLowerCase()
  const index = textLow.indexOf(search)
  if (index >= 0) {
    return escapeHtml(text.substring(0, index)) + newtext + escapeHtml(text.substring(index + search.length))
  } else return escapeHtml(text)
}

const toGlobalRegExp = (search: string | RegExp): RegExp => {
  if (typeof search === 'string') return new RegExp(search.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'g')
  return new RegExp(search.source, search.flags.includes('g') ? search.flags : search.flags + 'g')
}

// 生成高亮 HTML：未命中的原始文件名片段会被转义，命中部分由 replacer 决定（字符串 replacer 中的 $ 会被转义为字面量，避免 $& 等模式被展开）
const highlightReplace = (text: string, search: string | RegExp, replacer: string | ((match: string) => string), once = false): string => {
  const reg = toGlobalRegExp(search)
  const single = new RegExp(reg.source, reg.flags.replace(/g/g, ''))
  let out = ''
  let last = 0
  let done = false
  let m: RegExpExecArray | null
  while (!done && (m = reg.exec(text)) !== null) {
    out += escapeHtml(text.slice(last, m.index))
    out += typeof replacer === 'function' ? replacer(m[0]) : m[0].replace(single, replacer.replace(/\$/g, '$$$$'))
    last = m.index + m[0].length
    if (m[0] === '') reg.lastIndex++
    if (once) done = true
  }
  out += escapeHtml(text.slice(last))
  return out
}

function fixext(ext: string) {
  return ext ? '.' + ext : ''
}

function RunReplace(isDir: boolean, title: string, config: any) {
  const search = config.search as string
  if (!search) return [title, title]

  let name = title
  let ext = ''
  const exti = name.lastIndexOf('.')
  if (!isDir && exti >= 0) {
    ext = name.substring(exti + 1)
    name = name.substring(0, exti)
  }

  if (config.chkReg) {
    
    let reg: RegExp
    try {
      reg = RegExp(search)
    } catch {
      return [title, title] 
    }
    reg = RegExp(search)
    if (config.chkCase) reg = RegExp(search, 'i')
    if (config.chkAll) reg = RegExp(search, 'g')
    if (config.chkCase && config.chkAll) reg = RegExp(search, 'ig')
    if (config.applyto == 'full') {
      
      return [title.replace(reg, config.newword), highlightReplace(title, reg, '<s>' + config.newword + '</s>')]
    }
    if (config.applyto == 'name') {
      return [name.replace(reg, config.newword) + fixext(ext), highlightReplace(name, reg, '<s>' + config.newword + '</s>') + fixext(ext)]
    }
    if (config.applyto == 'ext') {
      return [name + fixext(ext.replace(reg, config.newword)), name + fixext(highlightReplace(ext, reg, '<s>' + config.newword + '</s>'))]
    }
  } else if (config.chkCase) {
    
    if (config.chkAll) {
      if (config.applyto == 'full') {
        const slist = Split(title, search)
        return [slist.join(config.newword), slist.map(escapeHtml).join('<s>' + config.newword + '</s>')]
      }
      if (config.applyto == 'name') {
        const slist = Split(name, search)
        return [slist.join(config.newword) + fixext(ext), slist.map(escapeHtml).join('<s>' + config.newword + '</s>') + fixext(ext)]
      }
      if (config.applyto == 'ext') {
        const slist = Split(ext, search)
        return [name + fixext(slist.join(config.newword)), name + fixext(slist.map(escapeHtml).join('<s>' + config.newword + '</s>'))]
      }
    } else {
      if (config.applyto == 'full') {
        return [Replace(title, search, config.newword), ReplaceHtml(title, search, '<s>' + config.newword + '</s>')]
      }
      if (config.applyto == 'name') {
        return [Replace(name, search, config.newword) + fixext(ext), ReplaceHtml(name, search, '<s>' + config.newword + '</s>') + fixext(ext)]
      }
      if (config.applyto == 'ext') {
        return [name + fixext(Replace(ext, search, config.newword)), name + fixext(ReplaceHtml(ext, search, '<s>' + config.newword + '</s>'))]
      }
    }
  } else {
    
    if (config.chkAll) {
      if (config.applyto == 'full') {
        return [title.replaceAll(search, config.newword), highlightReplace(title, search, '<s>' + config.newword + '</s>')]
      }
      if (config.applyto == 'name') {
        return [name.replaceAll(search, config.newword) + fixext(ext), highlightReplace(name, search, '<s>' + config.newword + '</s>') + fixext(ext)]
      }
      if (config.applyto == 'ext') {
        return [name + fixext(ext.replaceAll(search, config.newword)), name + fixext(highlightReplace(ext, search, '<s>' + config.newword + '</s>'))]
      }
    } else {
      if (config.applyto == 'full') {
        return [title.replace(search, config.newword), highlightReplace(title, search, '<s>' + config.newword + '</s>', true)]
      }
      if (config.applyto == 'name') {
        return [name.replace(search, config.newword) + fixext(ext), highlightReplace(name, search, '<s>' + config.newword + '</s>', true) + fixext(ext)]
      }
      if (config.applyto == 'ext') {
        return [name + fixext(ext.replace(search, config.newword)), name + fixext(highlightReplace(ext, search, '<s>' + config.newword + '</s>', true))]
      }
    }
  }
  return [title, title]
}
function RunDelete(isDir: boolean, title: string, config: any) {
  let name = title
  let ext = ''
  const exti = name.lastIndexOf('.')
  if (!isDir && exti >= 0) {
    ext = name.substring(exti + 1)
    name = name.substring(0, exti)
  }

  if (config.type == 'search' && config.search) {
    const search = config.search as string
    if (config.chkReg) {
      
      let reg: RegExp
      try {
        reg = RegExp(search)
      } catch {
        return [title, title] 
      }
      reg = RegExp(search)

      if (config.chkCase) reg = RegExp(search, 'i')
      if (config.chkAll) reg = RegExp(search, 'g')
      if (config.chkCase && config.chkAll) reg = RegExp(search, 'ig')
      if (config.applyto == 'full') {
        
        return [title.replace(reg, ''), highlightReplace(title, reg, (L) => (L ? '<b>' + escapeHtml(L) + '</b>' : ''))]
      }
      if (config.applyto == 'name') {
        return [name.replace(reg, '') + fixext(ext), highlightReplace(name, reg, (L) => (L ? '<b>' + escapeHtml(L) + '</b>' : '')) + fixext(ext)]
      }
      if (config.applyto === 'ext') {
        return [name + fixext(ext.replace(reg, '')), name + fixext(highlightReplace(ext, reg, (L) => (L ? '<b>' + escapeHtml(L) + '</b>' : '')))]
      }
    } else if (config.chkCase) {
      
      if (config.chkAll) {
        if (config.applyto == 'full') {
          const slist = Split(title, search)
          return [slist.join(''), slist.map(escapeHtml).join('<b>' + search + '</b>')]
        }
        if (config.applyto == 'name') {
          const slist = Split(name, search)
          return [slist.join('') + fixext(ext), slist.map(escapeHtml).join('<b>' + search + '</b>') + fixext(ext)]
        }
        if (config.applyto == 'ext') {
          const slist = Split(ext, search)
          return [name + fixext(slist.join('')), name + fixext(slist.map(escapeHtml).join('<b>' + search + '</b>'))]
        }
      } else {
        if (config.applyto == 'full') {
          return [Replace(title, search, ''), ReplaceHtml(title, search, '<b>' + search + '</b>')]
        }
        if (config.applyto == 'name') {
          return [Replace(name, search, '') + fixext(ext), ReplaceHtml(name, search, '<b>' + search + '</b>') + fixext(ext)]
        }
        if (config.applyto === 'ext') {
          return [name + fixext(Replace(ext, search, '')), name + fixext(ReplaceHtml(ext, search, '<b>' + search + '</b>'))]
        }
      }
    } else {
      
      if (config.chkAll) {
        if (config.applyto == 'full') {
          return [title.replaceAll(search, ''), highlightReplace(title, search, '<b>' + search + '</b>')]
        }
        if (config.applyto == 'name') {
          return [name.replaceAll(search, '') + fixext(ext), highlightReplace(name, search, '<b>' + search + '</b>') + fixext(ext)]
        }
        if (config.applyto == 'ext') {
          return [name + fixext(ext.replaceAll(search, '')), name + fixext(highlightReplace(ext, search, '<b>' + search + '</b>'))]
        }
      } else {
        if (config.applyto == 'full') {
          return [title.replace(search, ''), highlightReplace(title, search, '<b>' + search + '</b>', true)]
        }
        if (config.applyto == 'name') {
          return [name.replace(search, '') + fixext(ext), highlightReplace(name, search, '<b>' + search + '</b>', true) + fixext(ext)]
        }
        if (config.applyto == 'ext') {
          return [name + fixext(ext.replace(search, '')), name + fixext(highlightReplace(ext, search, '<b>' + search + '</b>', true))]
        }
      }
    }
  }

  if (config.type == 'position' && config.beginlen >= 0 && config.endlen >= 0) {
    let title1 = title
    let title2 = title
    if (config.applyto == 'full') {
      title1 = title
      title2 = title
    } else if (config.applyto == 'name') {
      title1 = name
      title2 = name
    } else if (config.applyto == 'ext') {
      title1 = ext
      title2 = ext
    }

    if (config.applyto == 'ext' && (config.beginlen > title1.length || config.endlen > title1.length || config.beginlen + config.endlen > title1.length)) return [title, title] 
    if (config.applyto !== 'ext' && (config.beginlen >= title1.length || config.endlen >= title1.length || config.beginlen + config.endlen >= title1.length)) return [title, title] 

    if (config.beginlen > 0 && config.endlen > 0) {
      
      const del1 = title1.substring(0, config.beginlen)
      const del2 = title1.substring(title1.length - config.endlen)
      let str = title1.substring(config.beginlen)
      str = str.substring(0, str.length - config.endlen)

      title1 = str
      title2 = '<b>' + escapeHtml(del1) + '</b>' + escapeHtml(str) + '<b>' + escapeHtml(del2) + '</b>'
    } else if (config.beginlen > 0) {
      
      const del = title1.substring(0, config.beginlen)
      const str = title1.substring(config.beginlen)
      title1 = str
      title2 = '<b>' + escapeHtml(del) + '</b>' + escapeHtml(str)
    } else if (config.endlen > 0) {
      
      const str1 = title1.substring(0, title1.length - config.endlen)
      const str2 = title1.substring(0, title1.length - config.endlen)
      const del = title1.substring(title1.length - config.endlen)
      title1 = str1
      title2 = escapeHtml(str2) + '<b>' + escapeHtml(del) + '</b>'
    } else {
      
      return [title, title]
    }

    if (config.applyto == 'full') {
      return [title1, title2]
    }
    if (config.applyto == 'name') {
      return [title1 + fixext(ext), title2 + fixext(ext)]
    }
    if (config.applyto == 'ext') {
      return [name + fixext(title1), name + fixext(title2)]
    }
  }

  if (config.type == 'range' && config.beginword && config.endword) {
    if (config.applyto == 'full') {
      const start = title.indexOf(config.beginword)
      const end = title.indexOf(config.endword, start + 1)
      if (start >= 0 && end >= 0 && start < end - 1) {
        const title1 = title.substring(0, start + 1) + title.substring(end)
        const title2 = escapeHtml(title.substring(0, start + 1)) + '<b>' + escapeHtml(title.substring(start + 1, end)) + '</b>' + escapeHtml(title.substring(end))
        return [title1, title2]
      }
    } else if (config.applyto == 'name') {
      const start = name.indexOf(config.beginword)
      const end = name.indexOf(config.endword, start + 1)
      if (start >= 0 && end >= 0 && start < end - 1) {
        const name1 = name.substring(0, start + 1) + name.substring(end)
        const name2 = escapeHtml(name.substring(0, start + 1)) + '<b>' + escapeHtml(name.substring(start + 1, end)) + '</b>' + escapeHtml(name.substring(end))
        return [name1 + fixext(ext), name2 + fixext(ext)]
      }
    } else if (config.applyto == 'ext') {
      const start = ext.indexOf(config.beginword)
      const end = ext.indexOf(config.endword, start + 1)
      if (start >= 0 && end >= 0 && start < end - 1) {
        const ext1 = ext.substring(0, start + 1) + ext.substring(end)
        const ext2 = escapeHtml(ext.substring(0, start + 1)) + '<b>' + escapeHtml(ext.substring(start + 1, end)) + '</b>' + escapeHtml(ext.substring(end))
        return [name + fixext(ext1), name + fixext(ext2)]
      }
    }
  }

  return [title, title]
}
function RunAdd(isDir: boolean, title: string, config: any) {
  let name = title
  let ext = ''
  const exti = name.lastIndexOf('.')
  if (!isDir && exti >= 0) {
    ext = name.substring(exti + 1)
    name = name.substring(0, exti)
  }

  let title1 = title

  if (config.applyto == 'full') {
    title1 = title
  } else if (config.applyto == 'name') {
    title1 = name
  } else if (config.applyto == 'ext') {
    title1 = ext
  }

  if (config.type == 'search' && config.search) {
    const index = title1.indexOf(config.search)
    if (index >= 0) {
      let title2 = ''
      const start = title1.substring(0, index)
      const mid = title1.substring(index, index + config.search.length)
      const end = title1.substring(index + config.search.length)

      if (config.before && config.after) {
        title1 = start + config.before + mid + config.after + end
        title2 = escapeHtml(start) + '<i>' + config.before + '</i>' + escapeHtml(mid) + '<i>' + config.after + '</i>' + escapeHtml(end)
      } else if (config.before) {
        title1 = start + config.before + mid + end
        title2 = escapeHtml(start) + '<i>' + config.before + '</i>' + escapeHtml(mid) + escapeHtml(end)
      } else if (config.after) {
        title1 = start + mid + config.after + end
        title2 = escapeHtml(start) + escapeHtml(mid) + '<i>' + config.after + '</i>' + escapeHtml(end)
      } else {
        return [title, title]
      }

      if (config.applyto == 'full') {
        return [title1, title2]
      }
      if (config.applyto == 'name') {
        return [title1 + fixext(ext), title2 + fixext(ext)]
      }
      if (config.applyto == 'ext') {
        return [name + fixext(title1), name + fixext(title2)]
      }
    }
  }

  if (config.type == 'position' && (config.beginword || config.endword)) {
    if (title1) {
      let title2 = escapeHtml(title1)
      if (config.beginword) {
        title1 = config.beginword + title1
        title2 = '<i>' + config.beginword + '</i>' + title2
      }
      if (config.endword) {
        title1 = title1 + config.endword
        title2 = title2 + '<i>' + config.endword + '</i>'
      }
      if (config.applyto == 'full') {
        return [title1, title2]
      }
      if (config.applyto == 'name') {
        return [title1 + fixext(ext), title2 + fixext(ext)]
      }
      if (config.applyto == 'ext') {
        return [name + fixext(title1), name + fixext(title2)]
      }
    }
  }

  return [title, title]
}
function RunIndex(isDir: boolean, title: string, config: any, nodeIndex: number) {
  if (!config.format) return [title, title]
  let name = title
  let ext = ''
  const exti = name.lastIndexOf('.')
  if (!isDir && exti >= 0) {
    ext = name.substring(exti)
    name = name.substring(0, exti)
  }

  const bianhao = config.beginindex + config.minnum * nodeIndex
  const formate = config.format.replace('#', bianhao.toString().padStart(config.minlen, '0'))
  if (config.type == 'begin') {
    const title1 = formate + name
    const title2 = '<i>' + formate + '</i>' + escapeHtml(name)
    return [title1 + ext, title2 + escapeHtml(ext)]
  } else if (config.type == 'end') {
    const title1 = name + formate
    const title2 = escapeHtml(name) + '<i>' + formate + '</i>'
    return [title1 + ext, title2 + escapeHtml(ext)]
  } else if (config.type == 'cover') {
    const title1 = formate
    const title2 = '<i>' + formate + '</i>'
    return [title1 + ext, title2 + escapeHtml(ext)]
  }

  return [title, title]
}
function RunOthers(isDir: boolean, title: string, config: any, sj1Base: string) {
  let name = title
  let ext = ''
  const exti = name.lastIndexOf('.')
  if (!isDir && exti >= 0) {
    ext = name.substring(exti)
    name = name.substring(0, exti)
  }

  if (config.nameformat == 'AA') {
    const title1 = name.replace(/[a-zA-Z]+/g, (L) => L.toUpperCase())
    const title2 = name.replace(/[a-zA-Z]+/g, (L) => (L == L.toUpperCase() ? escapeHtml(L) : '<s>' + escapeHtml(L.toUpperCase()) + '</s>'))

    if (title1 + ext == title) return [title, title] 
    return [title1 + ext, title2 + escapeHtml(ext)]
  }
  if (config.nameformat == 'aa') {
    const title1 = name.replace(/[a-zA-Z]+/g, (L) => L.toLowerCase())
    const title2 = name.replace(/[a-zA-Z]+/g, (L) => (L == L.toLowerCase() ? escapeHtml(L) : '<s>' + escapeHtml(L.toLowerCase()) + '</s>'))

    if (title1 + ext == title) return [title, title] 
    return [title1 + ext, title2 + escapeHtml(ext)]
  }
  if (config.nameformat == 'Aa') {
    name = name.toLowerCase()
    const title1 = name.replace(/[a-z]+/, (L) => (L.length > 1 ? L.substring(0, 1).toUpperCase() + L.substring(1) : L.toUpperCase()))
    const title2 = name.replace(/[a-z]+/, (L) => (L.length > 1 ? '<s>' + escapeHtml(L.substring(0, 1).toUpperCase()) + '</s>' + escapeHtml(L.substring(1)) : '<s>' + escapeHtml(L.toUpperCase()) + '</s>'))

    if (title1 + ext == title) return [title, title] 
    return [title1 + ext, title2 + escapeHtml(ext)]
  }
  if (config.nameformat == 'Aa Aa') {
    name = name.toLowerCase()
    const title1 = name.replace(/[a-z]+/g, (L) => (L.length > 1 ? L.substring(0, 1).toUpperCase() + L.substring(1) : L.toUpperCase()))
    const title2 = name.replace(/[a-z]+/g, (L) => (L.length > 1 ? '<s>' + escapeHtml(L.substring(0, 1).toUpperCase()) + '</s>' + escapeHtml(L.substring(1)) : '<s>' + escapeHtml(L.toUpperCase()) + '</s>'))
    if (title1 + ext == title) return [title, title] 
    return [title1 + ext, title2 + escapeHtml(ext)]
  }

  if (config.extformat == 'AA') {
    if (ext && ext != ext.toUpperCase()) {
      return [name + ext.toUpperCase(), escapeHtml(name) + '<s>' + escapeHtml(ext.toUpperCase()) + '</s>']
    } else {
      return [title, title] 
    }
  }
  if (config.extformat == 'aa') {
    if (ext && ext != ext.toLowerCase()) {
      return [name + ext.toLowerCase(), escapeHtml(name) + '<s>' + escapeHtml(ext.toLowerCase()) + '</s>']
    } else {
      return [title, title] 
    }
  }

  
  const randomLen = config.randomlen
  if (sj1Base && randomLen > 0) {
    let ranname = ''
    let ran = Math.random().toString() 
    ran = ran + ran + ran + ran + ran 
    let pos = name.length
    for (let i = 0; i < randomLen; i++) {
      pos = (pos ^ ran.charCodeAt(i)) % 300
      ranname += sj1Base[pos]
    }
    return [ranname + ext, '<s>' + ranname + '</s>' + escapeHtml(ext)]
  }
  return [title, title] 
}

export function RunReplaceName(renameConfig: any, treeData: TreeNodeData[], checkedKeys: string[]) {
  const show = renameConfig.show
  const replace = renameConfig.replace
  const checked = new Set(checkedKeys || [])
  if (replace.enable) {
    RunAllNode(treeData, (node) => {
      if (checked.has(node.key)) {
        const title = RunReplace(node.isDir, node.rawtitle, replace)
        node.newtitle = title[0]
        node.title = show ? title[0] : title[1]
        node.isMatch = node.newtitle != node.rawtitle
      } else {
        node.newtitle = node.rawtitle
        node.title = node.rawtitle
        node.isMatch = false
      }
      return true
    })
    return
  }
  const tdelete = renameConfig.delete
  if (tdelete.enable) {
    RunAllNode(treeData, (node) => {
      if (checked.has(node.key)) {
        const title = RunDelete(node.isDir, node.rawtitle, tdelete)
        node.newtitle = title[0]
        node.title = show ? title[0] : title[1]
        node.isMatch = node.newtitle != node.rawtitle && node.newtitle != ''
      } else {
        node.newtitle = node.rawtitle
        node.title = node.rawtitle
        node.isMatch = false
      }
      return true
    })
    return
  }
  const add = renameConfig.add
  if (add.enable) {
    RunAllNode(treeData, (node) => {
      if (checked.has(node.key)) {
        const title = RunAdd(node.isDir, node.rawtitle, add)
        node.newtitle = title[0]
        node.title = show ? title[0] : title[1]
        node.isMatch = node.newtitle != node.rawtitle
      } else {
        node.newtitle = node.rawtitle
        node.title = node.rawtitle
        node.isMatch = false
      }
      return true
    })
    return
  }
  const index = renameConfig.index
  if (index.enable) {
    let nodeindex = 0
    // const checkmap = new Set(checkedKeys || [])
    RunAllNode(treeData, (node) => {
      if (checked.has(node.key)) {
        const title = RunIndex(node.isDir, node.rawtitle, index, nodeindex)
        node.newtitle = title[0]
        node.title = show ? title[0] : title[1]
        node.isMatch = node.newtitle != node.rawtitle
        nodeindex++
      } else {
        node.newtitle = node.rawtitle
        node.title = node.rawtitle
        node.isMatch = false
      }
      return true
    })
    return
  }
  const others = renameConfig.others
  if (others.enable) {
    let sj1Base = '' 
    if (others.randomformat == '0-9') sj1Base = '01234567899876543210012345678998765432100123456789987654321001'
    if (others.randomformat == 'a-z') sj1Base = 'abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghij'
    if (others.randomformat == 'A-Z') sj1Base = 'ABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJKLMNOPQRSTUVWXYZABCDEFGHIJ'
    if (others.randomformat == 'a-zA-Z') sj1Base = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZtuvwxABCDE'
    if (others.randomformat == '0-9a-z') sj1Base = '0123456789abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnop'
    if (others.randomformat == '0-9A-Z') sj1Base = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEFGHIJKLMNOP'
    if (others.randomformat == '0-9a-zA-Z') sj1Base = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz'
    if (sj1Base) sj1Base = sj1Base + sj1Base + sj1Base + sj1Base + sj1Base 
    RunAllNode(treeData, (node) => {
      if (checked.has(node.key)) {
        const title = RunOthers(node.isDir, node.rawtitle, others, sj1Base)
        node.newtitle = title[0]
        node.title = show ? title[0] : title[1]
        node.isMatch = node.newtitle != node.rawtitle
      } else {
        node.newtitle = node.rawtitle
        node.title = node.rawtitle
        node.isMatch = false
      }
      return true
    })
  }
}
