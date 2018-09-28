package hangman

var (
	boards = []string{
` ___________.._______
| .__________))______|
| | / /
| |/ /
| | /
| |/
| |
| |
| |
| |
| |
| |
| |
| |
| |
| |
| |
| |
""""""""""""""""""""""""|
|"|"""""""""""""""""""""|
| |                   | |
: :                   : :
. .                   . .`,
` ___________.._______
| .__________))______|
| | / /      ||
| |/ /       ||
| | /        ||
| |/         ||
| |          ||
| |         /  \
| |         |  |
| |         |  |
| |         \__/
| |
| |
| |
| |
| |
| |
| |
""""""""""""""""""""""""|
|"|"""""""""""""""""""""|
| |                   | |
: :                   : :
. .                   . .`,
` ___________.._______
| .__________))______|
| | / /      ||
| |/ /       ||
| | /        ||.-''.
| |/         |/  _  \
| |          ||  '/,|
| |          (\\'_.'
| |
| |
| |
| |
| |
| |
| |
| |
| |
| |
""""""""""""""""""""""""|
|"|"""""""""""""""""""""|
| |                   | |
: :                   : :
. .                   . .`,
` ___________.._______
| .__________))______|
| | / /      ||
| |/ /       ||
| | /        ||.-''.
| |/         |/  _  \
| |          ||  '/,|
| |          (\\'_.'
| |          -'--'
| |          |. .|
| |          |   |
| |          | . |
| |          |   |
| |
| |
| |
| |
| |
""""""""""""""""""""""""|
|"|"""""""""""""""""""""|
| |                   | |
: :                   : :
. .                   . .`,
` ___________.._______
| .__________))______|
| | / /      ||
| |/ /       ||
| | /        ||.-''.
| |/         |/  _  \
| |          ||  '/,|
| |          (\\'_.'
| |         .-'--'
| |        /Y . .|
| |       // |   |
| |      //  | . |
| |     ')   |   |
| |
| |
| |
| |
| |
""""""""""""""""""""""""|
|"|"""""""""""""""""""""|
| |                   | |
: :                   : :
. .                   . .`,
` ___________.._______
| .__________))______|
| | / /      ||
| |/ /       ||
| | /        ||.-''.
| |/         |/  _  \
| |          ||  '/,|
| |          (\\'_.'
| |         .-'--'.
| |        /Y . . Y\
| |       // |   | \\
| |      //  | . |  \\
| |     ')   |   |   ('
| |
| |
| |
| |
| |
""""""""""""""""""""""""|
|"|"""""""""""""""""""""|
| |                   | |
: :                   : :
. .                   . .`,
` ___________.._______
| .__________))______|
| | / /      ||
| |/ /       ||
| | /        ||.-''.
| |/         |/  _  \
| |          ||  '/,|
| |          (\\'_.'
| |         .-'--'.
| |        /Y . . Y\
| |       // |   | \\
| |      //  | . |  \\
| |     ')   | __|   ('
| |          ||
| |          ||
| |          ||
| |          ||
| |         / |
"""""""""""|__|"""""""""|
|"|"""""""""""""""""""""|
| |                   | |
: :                   : :
. .                   . .`,
` ___________.._______
| .__________))______|
| | / /      ||
| |/ /       ||
| | /        ||.-''.
| |/         |/  _  \
| |          ||  '/,|
| |          (\\'_.'
| |         .-'--'.
| |        /Y . . Y\
| |       // |   | \\
| |      //  | . |  \\
| |     ')   | _ |   ('
| |          || ||
| |          || ||
| |          || ||
| |          || ||
| |         / | | \
"""""""""""|__|"|__|""""|
|"|"""""""""""""""""""""|
| |                   | |
: :                   : :
. .                   . .`,

` ___________.._______
| .__________))______|
| | / /      ||
| |/ /       ||
| | /        ||.-''.
| |/         |/  _  \
| |          ||  '/,|
| |          (\\'_.'
| |         .-'--'.
| |        /Y . . Y\
| |       // |   | \\
| |      //  | . |  \\
| |     ')   |   |   ('
| |          ||'||
| |          || ||
| |          || ||
| |          || ||
| |         / | | \
""""""""""|_'-' '-' |"""|
|"|"""""""\ \       '"|"|
| |        \ \        | |
: :         \ \       : :
. .          ''       . .`}
)