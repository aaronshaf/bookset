{{.Setup}}

#show footnote.entry: set block(above: 0.65em, below: 0.22em)
#show footnote.entry: set text(font: "{{.BodyFont}}", weight: "regular", size: 7pt)
// Typst's par.leading is the line-box gap; keep footnote text compact and
// use the block spacing above for separation from the preceding text.
#show footnote.entry: set par(leading: 4pt)

#show heading.where(level: 1): it => {
  block(width: 100%, above: 1.7em, below: 0.95em)[
    #align(center)[#text(font: "{{.HeadingFont}}", weight: "regular", size: 13.5pt)[#it.body]]
  ]
}

#show heading.where(level: 2): it => {
  block(width: 100%, above: 1.05em, below: 0.75em)[
    #text(font: "{{.HeadingFont}}", size: 13.5pt, weight: "bold")[#it.body]
  ]
}

#let chapter-title(title, chapter_label) = {
  set par(justify: false)
  pagebreak(weak: true)
  v(0.82in)
  align(center)[#text(font: "{{.UtilityFont}}", weight: "medium", size: 9.5pt, tracking: 0.15em, fill: rgb("555555"))[#upper(chapter_label)]]
  v(0.56in)
  align(center)[#box[#text(font: "{{.HeadingFont}}", weight: "bold", size: 25pt)[#title]]]
  v(0.60in)
}

#let running-head(p) = {
  if calc.even(p) {
    grid(
      columns: (1fr, auto, 1fr),
      align: (left, center, right),
      [#bookset-folio(p)],
      [#text(font: "{{.UtilityFont}}", size: 7.5pt, weight: "medium", tracking: 0.08em, fill: rgb("555555"))[#upper("{{.BookTitle}}")]],
      [],
    )
  } else {
    grid(
      columns: (1fr, auto, 1fr),
      align: (left, center, right),
      [],
      [#text(font: "{{.HeadingFont}}", size: 7.5pt, style: "italic", fill: rgb("555555"))[#bookset-chapter.get()]],
      [#bookset-folio(p)],
    )
  }
}

#set page(header: context {
  let p = counter(page).get().first()
  if bookset-running-heads.get() and p > 1 {
    v(0.18in)
    running-head(p)
  }
})

#let then-now(label, body) = {
  block(above: 0.38em, below: 0.78em)[
    #grid(
      columns: (0.65in, 1fr),
      gutter: 0.17in,
      align: (left, top),
      [#text(font: "{{.UtilityFont}}", weight: "bold", size: 10pt, tracking: 0.06em, fill: rgb("555555"))[#label]],
      [#box(stroke: (left: 0.35pt + rgb("c2c2c2")), inset: (left: 0.16in))[
        #set par(first-line-indent: 0pt)
        #set par(leading: 5.5pt)
        #text(font: "{{.BodyFont}}", weight: "regular", size: {{.BodySize}})[#body]
      ]],
    )
  ]
}

#let timeline-item(date, body) = {
  block(above: 0.45em, below: 0.45em)[
    #grid(
      columns: (0.86in, 1fr),
      gutter: 0.11in,
      align: (right, left),
      [#text(font: "{{.UtilityFont}}", weight: "medium", size: 8pt, tracking: 0.02em, fill: rgb("555555"))[#date]],
      [#box(stroke: (left: 0.45pt + rgb("c9c9c9")), inset: (left: 0.14in))[
        #set par(first-line-indent: 0pt)
        #set par(leading: 4pt)
        #text(font: "{{.BodyFont}}", size: {{.BodySize}}, weight: "regular")[#body]
      ]],
    )
  ]
}

{{.Content}}
