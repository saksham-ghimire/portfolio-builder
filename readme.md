# Portfolio Builder

Portfolio Builder is a command-line tool that lets you generate a static portfolio website from a simple configuration file and a pre-defined template. It's designed to be a fast and easy way for developers to create a professional-looking online presence without needing to write all the HTML and CSS from scratch.

## How It Works

The tool operates in two simple steps:

### Download a Template:

First, you choose a template and download its `config.yml` file. This file acts as a blueprint, allowing you to customize your site's content.

### Generate Your Site:

After you've edited the config.yml file with your personal details, run the tool again to generate a complete, static website in your chosen output directory.

## Getting Started

**Step 1 : Download the Template Configuration**
To get started, simply execute the portfolio-builder with the `--template` flag. You can use 0001 as an example template ID.

```
./portfolio-builder --template=0001
```

This command will download the `config.yml` file for template 0001 to your current directory.

**Step 2 : Customize Your Portfolio**
Open the newly created `config.yml` file in your favorite text editor. This file contains sections for base, pages, and collections, which you can populate with your own information.

**Important Note:** If your portfolio requires any external assets like images, ensure they are in the same directory where your output files will be generated. By default, the output directory is the current folder (.).

**Step 3: Generate Your Portfolio**
Once you've customized the `config.yml` file to your liking, run the `portfolio-builder` command again, this time without any flags.

```
./portfolio-builder
```

The tool will read your configuration, download the template files, and generate a complete, static portfolio website in your output directory.

## Advanced Usage

For more control, you can use the following optional flags:

- `--config <file-path>`: Use this flag to specify a different path for your configuration file if it's not named config.yml or is not in the current directory.

- `--output-dir <dir-path>`: Change the output directory of the generated site. The default is the current directory (.).

- `--help`: Display a full list of commands and examples.
